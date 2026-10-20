package com.yourdesk.android.video;

import android.view.Surface;
import java.io.ByteArrayOutputStream;
import java.util.concurrent.CountDownLatch;
import java.util.concurrent.TimeUnit;
import java.util.concurrent.atomic.AtomicBoolean;
import java.util.concurrent.atomic.AtomicInteger;

/** 使用真正的背景交接與 watchdog，原生引擎以可控制的阻塞替身驗證。 */
public final class AsyncVideoDecoderHostTest {
  private static final class Engine implements AsyncVideoDecoder.Engine {
    final CountDownLatch entered = new CountDownLatch(1), release = new CountDownLatch(1), closed = new CountDownLatch(1);
    volatile boolean block, started, awaitingKeyframe = true;
    volatile int delayMs;
    final AtomicInteger calls = new AtomicInteger();
    final AtomicInteger recoveries = new AtomicInteger(), resets = new AtomicInteger();
    volatile Thread owner;
    private void owner() {
      if (owner == null) owner = Thread.currentThread();
      if (owner != Thread.currentThread()) throw new AssertionError("原生資源在不同執行緒操作");
    }
    public int queue(EncodedVideoFrame frame, Surface surface) {
      owner(); calls.incrementAndGet(); entered.countDown();
      try { if (block) release.await(); if (delayMs > 0) Thread.sleep(delayMs); }
      catch (InterruptedException e) { Thread.currentThread().interrupt(); }
      started = true; awaitingKeyframe = false; return 1;
    }
    public int drainOutput() { owner(); return 0; }
    public boolean needsKeyframe() { owner(); return awaitingKeyframe; }
    public boolean isStarted() { owner(); return started; }
    public int failedWireCodec() { owner(); return 0; }
    public String backend() { owner(); return "替身硬解"; }
    public void recover() { owner(); recoveries.incrementAndGet(); awaitingKeyframe = true; }
    public void reset() { owner(); resets.incrementAndGet(); started = false; awaitingKeyframe = true; }
    public void resetSession() { reset(); }
    public void close() { owner(); closed.countDown(); }
  }
  public static void main(String[] args) throws Exception {
    blockedDriverAndSurfaceOwnership();
    overloadNeedsPressure();
    normalOutputAndSessionReset();
    startupOverflowPreservesNativeSession();
    surfaceResetTakesPriorityOverRecovery();
    System.out.println("AsyncVideoDecoderHostTest PASS：阻塞、期限、雙槽、Surface 所有權、過載與重連（原生引擎替身）");
  }

  private static void surfaceResetTakesPriorityOverRecovery() throws Exception {
    Engine engine=new Engine();engine.block=true;
    AsyncVideoDecoder decoder=new AsyncVideoDecoder(()->engine,TimeUnit.SECONDS.toNanos(2),new AtomicBoolean());
    AtomicBoolean released=new AtomicBoolean();
    decoder.queue(frame(true),new Surface());
    check(engine.entered.await(1,TimeUnit.SECONDS),"未開始原生解碼");
    decoder.resetAfter(()->released.set(true));
    decoder.queue(frame(true),new Surface());
    decoder.recover();
    engine.release.countDown();
    await(released::get,"缺幀恢復覆蓋 Surface 的完整重設");
    check(engine.resets.get()==1 && engine.recoveries.get()==0,"Surface 重設被降級為缺幀恢復");
    decoder.close();check(engine.closed.await(1,TimeUnit.SECONDS),"關閉未回收原生擁有者");
  }

  private static void startupOverflowPreservesNativeSession() throws Exception {
    Engine engine = new Engine(); engine.block = true;
    AsyncVideoDecoder decoder = new AsyncVideoDecoder(() -> engine, TimeUnit.SECONDS.toNanos(2), new AtomicBoolean());
    Surface surface = new Surface();
    decoder.queue(frame(true),surface);
    check(engine.entered.await(1,TimeUnit.SECONDS),"startup did not begin");
    decoder.queue(frame(false),surface); decoder.queue(frame(false),surface);
    engine.release.countDown();
    await(()->engine.recoveries.get()==1,"overflow did not recover native references");
    check(engine.resets.get()==0,"slow startup repeatedly destroys native session");
    for (int i=0;i<100;i++) { decoder.recover(); decoder.queue(frame(false),surface); }
    Thread.sleep(40);
    check(engine.recoveries.get()==1,"同一缺口的重複通知反覆清空解碼器");
    check(decoder.needsKeyframe(),"恢復完成後仍應等待 IDR");
    decoder.queue(frame(true),surface);
    AtomicInteger rendered = new AtomicInteger();
    await(()->{ rendered.addAndGet(Math.max(0,decoder.drainOutput())); return rendered.get()>0; },"next IDR did not recover output");
    decoder.close(); check(engine.closed.await(1,TimeUnit.SECONDS),"close did not release native owner");
  }
  private static void blockedDriverAndSurfaceOwnership() throws Exception {
    Engine engine = new Engine(); engine.block = true;
    AtomicBoolean stalled = new AtomicBoolean(), released = new AtomicBoolean();
    AsyncVideoDecoder decoder = new AsyncVideoDecoder(() -> engine, TimeUnit.MILLISECONDS.toNanos(150), stalled);
    decoder.queue(frame(true), new Surface());
    check(engine.entered.await(1, TimeUnit.SECONDS), "未開始解碼");
    long started = System.nanoTime();
    for (int i = 0; i < 10000; i++) decoder.queue(frame(false), new Surface());
    decoder.resetAfter(() -> released.set(true));
    decoder.resetSession(); decoder.drainOutput(); decoder.close();
    check(System.nanoTime() - started < TimeUnit.SECONDS.toNanos(1), "UI 等待原生呼叫");
    await(stalled::get, "沒有偵測原生逾時");
    check(!released.get() && engine.closed.getCount() == 1, "驅動仍執行時提前釋放 Surface／codec");
    AtomicInteger factories = new AtomicInteger();
    AsyncVideoDecoder next = new AsyncVideoDecoder(() -> { factories.incrementAndGet(); return new Engine(); }, TimeUnit.SECONDS.toNanos(1), stalled);
    check(next.queue(frame(true), new Surface()) < 0 && factories.get() == 0, "持續建立停滯後端");
    next.close();
    engine.release.countDown();
    await(released::get, "原生返回後未釋放 Surface");
    check(engine.closed.getCount() == 0 && engine.calls.get() == 1, "重播了過期影格或未回收資源");
  }
  private static void overloadNeedsPressure() throws Exception {
    for (boolean pressure : new boolean[]{false, true}) {
      Engine engine = new Engine(); engine.delayMs = 140;
      AsyncVideoDecoder decoder = new AsyncVideoDecoder(() -> engine, TimeUnit.SECONDS.toNanos(2), new AtomicBoolean());
      for (int i = 1; i <= 4; i++) {
        decoder.queue(frame(true), new Surface()); final int count = i;
        await(() -> engine.calls.get() >= count, "未交接影格");
        if (pressure) decoder.noteDroppedFrame();
        Thread.sleep(170);
        decoder.drainOutput();
      }
      check(((decoder.failedCodecMask() & 2) != 0) == pressure, "過載未切換，或閒置慢速被誤判");
      decoder.close(); check(engine.closed.await(1, TimeUnit.SECONDS), "關閉未回收");
    }
  }
  private static void normalOutputAndSessionReset() throws Exception {
    Engine engine = new Engine();
    AsyncVideoDecoder decoder = new AsyncVideoDecoder(() -> engine, TimeUnit.SECONDS.toNanos(1), new AtomicBoolean());
    decoder.queue(frame(true), new Surface());
    await(decoder::isStarted, "正常解碼未完成");
    check(decoder.drainOutput() == 1 && !decoder.needsKeyframe(), "輸出快照不正確");
    decoder.resetSession();
    check(decoder.needsKeyframe() && decoder.drainOutput() == 0, "重連保留了舊輸出");
    decoder.close(); check(engine.closed.await(1, TimeUnit.SECONDS), "關閉未回收");
  }
  private interface Condition { boolean get(); }
  private static void await(Condition condition, String message) throws Exception {
    long deadline = System.nanoTime() + TimeUnit.SECONDS.toNanos(2);
    while (!condition.get() && System.nanoTime() < deadline) Thread.sleep(5);
    check(condition.get(), message);
  }
  private static void check(boolean ok, String message) { if (!ok) throw new AssertionError(message); }
  private static EncodedVideoFrame frame(boolean keyframe) {
    ByteArrayOutputStream out = new ByteArrayOutputStream(); out.write(2);
    for (byte[] nal : new byte[][]{{0x67, 100, 0, 40}, {0x68, 1}, {(byte)(keyframe ? 0x65 : 0x41), 1}}) {
      out.write(0); out.write(0); out.write(0); out.write(nal.length); out.write(nal, 0, nal.length);
    }
    return EncodedVideoFrame.parse(1, 1920, 1080, out.toByteArray(), keyframe);
  }
}
