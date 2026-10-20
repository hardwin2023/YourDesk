package com.yourdesk.android.video;

import android.view.Surface;
import java.util.concurrent.Executors;
import java.util.concurrent.ScheduledExecutorService;
import java.util.concurrent.ScheduledFuture;
import java.util.concurrent.TimeUnit;
import java.util.concurrent.atomic.AtomicBoolean;
import java.util.ArrayDeque;
import java.util.ArrayList;

/** UI 只交接工作／取快照；原生建立、解碼、重設與釋放均由同一工作者執行。 */
public final class AsyncVideoDecoder implements AutoCloseable {
  interface Engine extends AutoCloseable {
    int queue(EncodedVideoFrame frame, Surface surface);
    int drainOutput();
    boolean needsKeyframe();
    boolean isStarted();
    int failedWireCodec();
    String backend();
    default void recover() { reset(); }
    void reset();
    void resetSession();
    @Override void close();
  }
  interface Factory { Engine create(); }
  private static final class NativeEngine implements Engine {
    private final MediaCodecVideoDecoder decoder = new MediaCodecVideoDecoder();
    public int queue(EncodedVideoFrame frame, Surface surface) { return decoder.queue(frame, surface); }
    public int drainOutput() { return decoder.drainOutput(); }
    public boolean needsKeyframe() { return decoder.needsKeyframe(); }
    public boolean isStarted() { return decoder.isStarted(); }
    public int failedWireCodec() { return decoder.failedWireCodec(); }
    public String backend() { return decoder.backend(); }
    public void recover() { decoder.recover(); }
    public void reset() { decoder.reset(); }
    public void resetSession() { decoder.resetSession(); }
    public void close() { decoder.close(); }
  }
  private static final AtomicBoolean DRIVER_STALLED = new AtomicBoolean();
  private static final ScheduledExecutorService WATCHDOG = Executors.newSingleThreadScheduledExecutor(r -> {
    Thread t = new Thread(r, "YourDesk-video-watchdog"); t.setDaemon(true); return t;
  });
  private static final int VIDEO_MASK = (1 << 1) | (1 << 2);
  private static final java.util.concurrent.ExecutorService RELEASES = Executors.newSingleThreadExecutor(r -> {
    Thread t = new Thread(r, "YourDesk-surface-release"); t.setDaemon(true); return t;
  });
  private final Object lock = new Object();
  private final AtomicBoolean stalled;
  private final ScheduledFuture<?> watchdog;
  private final long timeoutNanos;
  private final ArrayDeque<Job> jobs = new ArrayDeque<>(2);
  private final ArrayDeque<Runnable> releases = new ArrayDeque<>();
  private boolean finished;
  private boolean closed, retired, resetRequested, sessionResetRequested, active;
  private boolean recoveryOnly;
  private boolean acceptingDelta, awaitingKeyframe = true, started, failurePending;
  private long generation, session, callStarted, drops;
  private int rendered, failedMask, currentCodec;
  private String backend = "";
  private final boolean[] warmed = new boolean[3];
  private final int[] slow = new int[3];
  private final long[] observedDrops = new long[3];
  private static final class Job {
    final EncodedVideoFrame frame;
    final Surface surface;
    final long generation, session;
    Job(EncodedVideoFrame f, Surface s, long g, long e) { frame = f; surface = s; generation = g; session = e; }
  }

  public AsyncVideoDecoder() { this(NativeEngine::new, TimeUnit.SECONDS.toNanos(5), DRIVER_STALLED); }
  AsyncVideoDecoder(Factory factory, long timeoutNanos, AtomicBoolean stalled) {
    this.timeoutNanos = timeoutNanos; this.stalled = stalled;
    if (stalled.get()) { retired = finished = true; failedMask = VIDEO_MASK; watchdog = null; return; }
    watchdog = WATCHDOG.scheduleWithFixedDelay(this::checkTimeout, 50, 50, TimeUnit.MILLISECONDS);
    Thread worker = new Thread(() -> run(factory), "YourDesk-video-decoder");
    worker.setDaemon(true); worker.start();
  }

  /** 包含處理中的影格，最多兩個槽位；不覆寫相依幀，滿載後從 IDR 恢復。 */
  public int queue(EncodedVideoFrame frame, Surface surface) {
    synchronized (lock) {
      if (frame == null || frame.codec < 1 || frame.codec > 2 || surface == null) return -1;
      if (closed || retired || (failedMask & (1 << frame.codec)) != 0) return -1;
      if (jobs.size() >= (active ? 1 : 2)) {
        drops++;
        recoverLocked();
      }
      if (!acceptingDelta && !frame.keyframe) return 0;
      acceptingDelta = true;
      jobs.addLast(new Job(frame, surface, generation, session));
      lock.notifyAll();
      return 0;
    }
  }
  public int drainOutput() {
    synchronized (lock) {
      if (failurePending) { failurePending = false; return -1; }
      int count = rendered; rendered = 0; return count;
    }
  }
  public boolean needsKeyframe() { synchronized (lock) { return awaitingKeyframe; } }
  public boolean isStarted() { synchronized (lock) { return started; } }
  public String backend() { synchronized (lock) { return backend; } }
  public int wireCodec() { synchronized (lock) { return currentCodec; } }
  public int failedCodecMask() { synchronized (lock) { return failedMask; } }
  public void noteDroppedFrame() { synchronized (lock) { drops++; } }
  public void recover() { synchronized (lock) { recoverLocked(); } }
  public void reset() { synchronized (lock) { resetLocked(false); } }
  public void resetSession() { synchronized (lock) { resetLocked(true); } }
  /** TextureView 回傳 false 後，延後至原生 renderer 停止才釋放 SurfaceTexture。 */
  public void resetAfter(Runnable release) {
    synchronized (lock) {
      if (finished) { RELEASES.execute(release); return; }
      releases.add(release); resetLocked(false);
    }
  }
  private void releaseSurfaces(boolean finishing) {
    ArrayList<Runnable> ready;
    synchronized (lock) {
      ready = new ArrayList<>(releases); releases.clear();
      if (finishing) finished = true;
    }
    for (Runnable release : ready) { try { release.run(); } catch (RuntimeException ignored) { } }
  }
  private void resetLocked(boolean newSession) {
    generation++; jobs.clear(); rendered = 0; started = false;
    acceptingDelta = false; awaitingKeyframe = true;
    resetRequested = true; sessionResetRequested |= newSession;
    recoveryOnly = false;
    if (newSession) {
      session++; failedMask = retired ? VIDEO_MASK : 0; failurePending = false; drops = 0;
      java.util.Arrays.fill(warmed, false); java.util.Arrays.fill(slow, 0); java.util.Arrays.fill(observedDrops, 0);
    }
    lock.notifyAll();
  }

  private void recoverLocked() {
    // 等候 IDR 期間的每個相依幀都可能帶來同一缺口通知，不重複 flush。
    // 首次恢復已淘汰舊工作；新的 IDR 入列後才可能需要下一次恢復。
    if (!acceptingDelta) return;
    // 缺幀不重新配置硬體；否則初始化慢於兩幀時會永遠卡在建立／銷毀循環。
    // 若已有 Surface／工作階段重設，仍保留較強的重設與資源回收要求。
    boolean hardPending = resetRequested && !recoveryOnly;
    resetLocked(false);
    recoveryOnly = !hardPending;
  }

  private void checkTimeout() {
    synchronized (lock) {
      if (callStarted == 0 || System.nanoTime() - callStarted < timeoutNanos) return;
      // 不在 watchdog 強制呼叫 release；仍由尚在原生 API 內的擁有者回收。
      stalled.set(true); retired = true; failedMask = VIDEO_MASK; failurePending = true;
      generation++; jobs.clear(); rendered = 0; started = false; awaitingKeyframe = true;
      lock.notifyAll(); watchdog.cancel(false);
    }
  }

  private void run(Factory factory) {
    Engine engine = null;
    try {
      synchronized (lock) { callStarted = System.nanoTime(); }
      engine = factory.create();
      synchronized (lock) { callStarted = 0; }
      for (;;) {
        Job job;
        boolean reset, newSession, recover;
        long epoch, sessionEpoch;
        synchronized (lock) {
          if (closed || retired) break;
          if (jobs.isEmpty() && !resetRequested) {
            lock.wait(started ? 16 : 0);
            if (closed || retired) break;
          }
          reset = resetRequested; newSession = sessionResetRequested;
          recover = recoveryOnly;
          resetRequested = sessionResetRequested = false;
          recoveryOnly = false;
          job = reset ? null : jobs.pollFirst();
          epoch = generation; sessionEpoch = session;
          active = true; callStarted = System.nanoTime();
        }
        int count = 0;
        boolean needKeyframe = true, running = false;
        int failed = 0;
        String name = "";
        long cost;
        try {
          if (reset) {
            if (newSession) engine.resetSession(); else if (recover) engine.recover(); else engine.reset();
            if (!recover) releaseSurfaces(false);
          }
          else if (job != null) count = engine.queue(job.frame, job.surface);
          else count = engine.drainOutput();
          needKeyframe = engine.needsKeyframe(); running = engine.isStarted();
          failed = engine.failedWireCodec(); name = engine.backend();
        } catch (RuntimeException | LinkageError e) {
          count = -1;
          failed = job == null ? currentCodec : job.frame.codec;
        }
        synchronized (lock) {
          cost = System.nanoTime() - callStarted; callStarted = 0; active = false;
          if (closed || retired) break;
          if (job != null && job.session == session) observeLoad(job.frame.codec, cost);
          if (epoch != generation || sessionEpoch != session) continue;
          if (job != null) currentCodec = job.frame.codec;
          if (failed > 0 && failed <= 2) failedMask |= 1 << failed;
          if (count < 0) failurePending = true; else rendered += count;
          awaitingKeyframe = needKeyframe; started = running; backend = name;
        }
      }
    } catch (InterruptedException e) {
      Thread.currentThread().interrupt();
    } catch (RuntimeException | LinkageError e) {
      synchronized (lock) { failedMask = VIDEO_MASK; failurePending = true; retired = true; }
    } finally {
      synchronized (lock) { callStarted = System.nanoTime(); }
      try { if (engine != null) engine.close(); releaseSurfaces(true); }
      finally { synchronized (lock) { callStarted = 0; active = false; } if (watchdog != null) watchdog.cancel(false); }
    }
  }

  private void observeLoad(int codec, long cost) {
    boolean pressure = drops > observedDrops[codec]; observedDrops[codec] = drops;
    if (!warmed[codec]) { warmed[codec] = true; return; }
    if (cost < TimeUnit.MILLISECONDS.toNanos(120) || !pressure) { slow[codec] = 0; return; }
    if (++slow[codec] < 3) return;
    failedMask |= 1 << codec; failurePending = true; resetLocked(false);
  }
  @Override public void close() {
    synchronized (lock) {
      closed = true; generation++; jobs.clear(); rendered = 0; started = false;
      lock.notifyAll();
    }
  }
}
