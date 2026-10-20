package com.yourdesk.android;

import static org.junit.Assert.*;
import static org.junit.Assume.assumeTrue;
import android.app.Instrumentation;
import android.content.Intent;
import android.graphics.Bitmap;
import android.graphics.Color;
import android.graphics.SurfaceTexture;
import android.os.Bundle;
import android.os.SystemClock;
import android.view.View;
import android.view.MotionEvent;
import android.view.InputDevice;
import android.view.TextureView;
import android.webkit.WebView;
import androidx.test.ext.junit.runners.AndroidJUnit4;
import androidx.test.platform.app.InstrumentationRegistry;
import com.yourdesk.androidcore.core.Viewer;
import com.yourdesk.android.audio.RemoteAudioSession;
import com.yourdesk.android.video.AsyncVideoDecoder;
import com.yourdesk.android.video.DecoderSupport;
import java.lang.reflect.Field;
import java.lang.reflect.Method;
import java.util.concurrent.CountDownLatch;
import java.util.concurrent.TimeUnit;
import java.util.concurrent.atomic.AtomicReference;
import java.util.concurrent.atomic.AtomicInteger;
import org.json.JSONObject;
import org.junit.After;
import org.junit.Before;
import org.junit.Test;
import org.junit.runner.RunWith;

/** 指定 smokeHost／smokeSecret 時，透過實際 TLS／P2P／JNI／WebView 驗證 LAN 連線。 */
@RunWith(AndroidJUnit4.class)
public class NetworkSmokeTest {
  private final Instrumentation instrumentation = InstrumentationRegistry.getInstrumentation();
  private MainActivity activity;
  private String host, secret;
  @Before public void open() throws Exception {
    Bundle args = InstrumentationRegistry.getArguments();
    host = args.getString("smokeHost", ""); secret = args.getString("smokeSecret", "");
    assumeTrue("需啟動 android/tests/smoke-host 並提供測試參數", !host.isEmpty() && !secret.isEmpty());
    activity = (MainActivity) instrumentation.startActivitySync(new Intent(instrumentation.getTargetContext(), MainActivity.class)
        .addFlags(Intent.FLAG_ACTIVITY_NEW_TASK));
    audioEnabled(false);
    waitJS("typeof openConnection==='function'");
  }
  @After public void close() {
    if(activity!=null) { audioEnabled(false); instrumentation.runOnMainSync(activity::finish); }
  }
  private Object field(String name) throws Exception {
    Field field = MainActivity.class.getDeclaredField(name); field.setAccessible(true); return field.get(activity);
  }
  private String js(String code) throws Exception {
    CountDownLatch done = new CountDownLatch(1);
    AtomicReference<String> value = new AtomicReference<>();
    instrumentation.runOnMainSync(() -> {
      try { ((WebView)field("web")).evaluateJavascript(code, result -> {value.set(result);done.countDown();}); }
      catch(Exception error){throw new AssertionError(error);}
    });
    if(!done.await(2,TimeUnit.SECONDS))throw new java.util.concurrent.TimeoutException("頁面切換期間 WebView 未回覆");
    return value.get();
  }
  private void waitJS(String condition) throws Exception {
    long deadline=SystemClock.uptimeMillis()+45000;
    while(SystemClock.uptimeMillis()<deadline) {
      // navigation 可能取消尚未完成的 evaluateJavascript；等待新 document 再確認條件。
      try { if("true".equals(js(condition)))return; }
      catch(java.util.concurrent.TimeoutException loading) { }
      SystemClock.sleep(100);
    }
    fail("連線 Smoke 逾時："+condition+"；頁面="+js("location.pathname+' '+document.body.innerText.slice(0,600)"));
  }
  private void connect(String mode) throws Exception {
    js("openConnection("+JSONObject.quote(host)+","+JSONObject.quote(mode)+");document.getElementById('connect-secret').value="
        +JSONObject.quote(secret)+";document.getElementById('connection-form').dispatchEvent(new Event('submit',{cancelable:true}));");
  }
  private int centerPixel() {
    int[] pixel={Color.BLACK};
    instrumentation.runOnMainSync(()->{
      try { Bitmap bitmap=(Bitmap)field("composedFrame");if(bitmap!=null)pixel[0]=bitmap.getPixel(bitmap.getWidth()/2,bitmap.getHeight()/2); }
      catch(Exception error){throw new AssertionError(error);}
    });
    return pixel[0];
  }

  private void command(String name) throws Exception {
    Viewer session=(Viewer)field("viewer");
    // Host 的原生輸入 worker 與命令 worker 分開；先等待控制佇列的同步點。
    if(name.startsWith("smoke.input.")) {
      session.sendControlJSON("{\"type\":\"smoke-barrier\"}");session.callCommand("smoke.barrier");
    }
    session.callCommand(name);
  }
  private void waitField(String name, Object expected) throws Exception {
    long deadline=SystemClock.uptimeMillis()+10000;
    while(SystemClock.uptimeMillis()<deadline) {
      AtomicReference<Object> result=new AtomicReference<>();
      instrumentation.runOnMainSync(()->{try{result.set(field(name));}catch(Exception e){throw new AssertionError(e);}});
      if(expected.equals(result.get()))return;
      SystemClock.sleep(20);
    }
    saveScreenshot("wait-"+name);
    fail("等待欄位逾時："+name+"="+field(name));
  }
  private void saveScreenshot(String name) throws Exception {
    Bitmap shot=instrumentation.getUiAutomation().takeScreenshot();
    java.io.File dir=new java.io.File(instrumentation.getTargetContext().getExternalFilesDir(null),"pointer-smoke");dir.mkdirs();
    if(shot!=null)try(java.io.FileOutputStream out=new java.io.FileOutputStream(new java.io.File(dir,name+".png"))){shot.compress(Bitmap.CompressFormat.PNG,100,out);}finally{shot.recycle();}
  }
  private void touch(long down, int action, float yFraction) {
    instrumentation.runOnMainSync(()->{
      try {
        View view=(View)field("nativeScreen");
        MotionEvent event=MotionEvent.obtain(down,SystemClock.uptimeMillis(),action,view.getWidth()*.6f,view.getHeight()*yFraction,0);
        event.setSource(InputDevice.SOURCE_TOUCHSCREEN);
        try{view.dispatchTouchEvent(event);}finally{event.recycle();}
      }catch(Exception e){throw new AssertionError(e);}
    });
  }
  private void tapDesktop() {long now=SystemClock.uptimeMillis();touch(now,MotionEvent.ACTION_DOWN,.5f);touch(now,MotionEvent.ACTION_UP,.5f);}
  private void longPressDesktop() {
    long now=SystemClock.uptimeMillis();touch(now,MotionEvent.ACTION_DOWN,.5f);
    SystemClock.sleep(android.view.ViewConfiguration.getLongPressTimeout()+150);
    touch(now,MotionEvent.ACTION_UP,.5f);
  }
  private void pinchDesktop() {
    instrumentation.runOnMainSync(()->{
      try {
        View view=(View)field("nativeScreen");
        float cx=view.getWidth()*.6f,cy=view.getHeight()*.5f,gap=view.getWidth()*.06f;
        int[] actions={MotionEvent.ACTION_DOWN,MotionEvent.ACTION_POINTER_DOWN|(1<<MotionEvent.ACTION_POINTER_INDEX_SHIFT),
            MotionEvent.ACTION_MOVE,MotionEvent.ACTION_POINTER_UP|(1<<MotionEvent.ACTION_POINTER_INDEX_SHIFT),MotionEvent.ACTION_UP};
        long now=SystemClock.uptimeMillis();
        for(int step=0;step<actions.length;step++) {
          int count=step==0||step==4?1:2;
          MotionEvent.PointerProperties[] p=new MotionEvent.PointerProperties[count];MotionEvent.PointerCoords[] c=new MotionEvent.PointerCoords[count];
          for(int i=0;i<count;i++) {
            p[i]=new MotionEvent.PointerProperties();p[i].id=i;p[i].toolType=MotionEvent.TOOL_TYPE_FINGER;
            c[i]=new MotionEvent.PointerCoords();c[i].x=cx+(i==0?-1:1)*gap*(step>=2?2:1);c[i].y=cy;c[i].pressure=1;
          }
          MotionEvent event=MotionEvent.obtain(now,now+step,actions[step],count,p,c,0,0,1,1,0,0,InputDevice.SOURCE_TOUCHSCREEN,0);
          try{view.dispatchTouchEvent(event);}finally{event.recycle();}
        }
        assertEquals(2f,((DesktopViewport)field("desktopViewport")).zoom(),.01f);
      }catch(Exception e){throw new AssertionError(e);}
    });
  }
  private void chooseViewport(String title) throws Exception {
    NativeMenuTestHelper.choose(instrumentation,activity,(View)field("viewportButton"),title);
  }
  private void mouse(int action, int buttons, float wheel) {
    instrumentation.runOnMainSync(()->{
      try {
        View view=(View)field("nativeScreen");
        MotionEvent.PointerProperties p=new MotionEvent.PointerProperties();p.id=0;p.toolType=MotionEvent.TOOL_TYPE_MOUSE;
        MotionEvent.PointerCoords c=new MotionEvent.PointerCoords();c.x=view.getWidth()*.6f;c.y=view.getHeight()*.5f;c.pressure=1;c.setAxisValue(MotionEvent.AXIS_VSCROLL,wheel);
        long now=SystemClock.uptimeMillis();
        MotionEvent event=MotionEvent.obtain(now,now,action,1,new MotionEvent.PointerProperties[]{p},new MotionEvent.PointerCoords[]{c},0,buttons,1,1,0,0,InputDevice.SOURCE_MOUSE,0);
        try {
          if(action==MotionEvent.ACTION_DOWN||action==MotionEvent.ACTION_UP||action==MotionEvent.ACTION_MOVE||action==MotionEvent.ACTION_CANCEL)view.dispatchTouchEvent(event);
          else view.dispatchGenericMotionEvent(event);
        }finally{event.recycle();}
      }catch(Exception e){throw new AssertionError(e);}
    });
  }

  @Test public void touchRightClickAndRemoteScrollKeepViewportIndependent() throws Exception {
    connect("desktop");waitJS("document.getElementById('display-select')?.options.length===3");waitField("displayInputBlocked",false);
    command("smoke.input.reset");longPressDesktop();command("smoke.input.button2");
    command("smoke.input.reset");tapDesktop();command("smoke.input.button1");
    command("smoke.input.reset");
    long now=SystemClock.uptimeMillis();touch(now,MotionEvent.ACTION_DOWN,.5f);touch(now,MotionEvent.ACTION_CANCEL,.5f);
    SystemClock.sleep(android.view.ViewConfiguration.getLongPressTimeout()+80);command("smoke.input.clean");
    chooseViewport("單指捲動遠端內容");waitField("remoteScrollMode",true);
    command("smoke.input.reset");now=SystemClock.uptimeMillis();touch(now,MotionEvent.ACTION_DOWN,.7f);touch(now,MotionEvent.ACTION_MOVE,.3f);touch(now,MotionEvent.ACTION_UP,.3f);
    command("smoke.input.wheel-down");
    command("smoke.input.reset");now=SystemClock.uptimeMillis();touch(now,MotionEvent.ACTION_DOWN,.3f);touch(now,MotionEvent.ACTION_MOVE,.7f);touch(now,MotionEvent.ACTION_UP,.7f);
    command("smoke.input.wheel-up");
    instrumentation.runOnMainSync(()->{try{assertEquals(1f,((DesktopViewport)field("desktopViewport")).zoom(),0);}catch(Exception e){throw new AssertionError(e);}});
    chooseViewport("單指拖移可視區");waitField("remoteScrollMode",false);
    chooseViewport("單指拖移可視區");waitField("viewportPanMode",false);
    command("smoke.input.reset");tapDesktop();command("smoke.input.button1");
  }

  @Test public void externalMousePairsButtonsAndAccumulatesWheel() throws Exception {
    connect("desktop");waitJS("document.getElementById('display-select')?.options.length===3");waitField("displayInputBlocked",false);
    for(int button:new int[]{MotionEvent.BUTTON_PRIMARY,MotionEvent.BUTTON_SECONDARY,MotionEvent.BUTTON_TERTIARY}) {
      command("smoke.input.reset");
      mouse(MotionEvent.ACTION_DOWN,button,0);mouse(MotionEvent.ACTION_BUTTON_PRESS,button,0);
      mouse(MotionEvent.ACTION_BUTTON_RELEASE,0,0);mouse(MotionEvent.ACTION_UP,0,0);
      command("smoke.input.button"+(button==MotionEvent.BUTTON_PRIMARY?1:button==MotionEvent.BUTTON_SECONDARY?2:3));
    }
    command("smoke.input.reset");mouse(MotionEvent.ACTION_SCROLL,0,.25f);command("smoke.input.clean");
    mouse(MotionEvent.ACTION_SCROLL,0,.75f);command("smoke.input.wheel-up");
    command("smoke.input.reset");mouse(MotionEvent.ACTION_SCROLL,0,-1);command("smoke.input.wheel-down");
    command("smoke.input.reset");mouse(MotionEvent.ACTION_DOWN,MotionEvent.BUTTON_SECONDARY,0);mouse(MotionEvent.ACTION_CANCEL,0,0);command("smoke.input.button2");
  }

  @Test public void displaySelectionWaitsForMatchingFrameAndHandlesRemoval() throws Exception {
    connect("desktop");waitJS("document.getElementById('display-select')?.options.length===3");
    command("smoke.input.reset");
    pinchDesktop();command("smoke.input.clean");
    js("displaySelect.value='1';displaySelect.dispatchEvent(new Event('change'))");
    waitField("displayInputBlocked",true);tapDesktop();
    assertTrue(activity.new Bridge().sendControlJSON("{\"type\":\"text\",\"text\":\"切換期間\"}").contains("切換中"));
    waitField("hostDisplay",1);
    assertEquals("確認已到，但舊影像不得當成新螢幕",0,field("viewportDisplay"));
    command("smoke.input.clean");
    waitField("viewportDisplay",1);waitField("displayInputBlocked",false);
    assertTrue("第二螢幕合成影像為黃色",Color.red(centerPixel())>180&&Color.green(centerPixel())>150);
    instrumentation.runOnMainSync(()->{try{assertEquals("切換後恢復完整可視區",1f,((DesktopViewport)field("desktopViewport")).zoom(),0);}catch(Exception e){throw new AssertionError(e);}});
    longPressDesktop();command("smoke.input.button2");
    js("displaySelect.value='2';displaySelect.dispatchEvent(new Event('change'))");waitField("viewportDisplay",2);waitField("displayInputBlocked",false);
    assertTrue("第三螢幕合成影像為紫色",Color.red(centerPixel())>150&&Color.blue(centerPixel())>150);
    command("smoke.displays.remove");waitField("viewportDisplay",0);
    waitJS("displaySelect.options.length===1 && displaySelect.disabled && displaySelect.value==='0'");
    command("smoke.displays.none");waitField("displayInputBlocked",true);
    waitJS("displaySelect.disabled && displaySelect.options[0].textContent==='沒有可用螢幕'");
    command("smoke.input.reset");tapDesktop();command("smoke.input.clean");
  }
  @Test public void desktopReceivesFramesSendsUnicodeAndReturnsHomeOnDisconnect() throws Exception {
    connect("desktop");waitJS("location.pathname.endsWith('/desktop.html')");
    assertTrue("實際 JPEG 影像應為藍色",Color.blue(centerPixel())>150);
    assertEquals("ok",activity.new Bridge().sendControlJSON("{\"type\":\"text\",\"text\":\"Android Smoke 中文\"}"));
    long deadline=SystemClock.uptimeMillis()+10000;
    while(SystemClock.uptimeMillis()<deadline && Color.red(centerPixel())<180)SystemClock.sleep(100);
    assertTrue("Host 收到中文控制訊息後應送回紅色影像",Color.red(centerPixel())>180);
    ((Viewer)field("viewer")).callCommand("session.disconnect");
    waitJS("location.pathname.endsWith('/index.html') && document.body.innerText.includes('遠端已斷線')");
  }
  @Test public void shellExchangesUtf8AndCanReconnect() throws Exception {
    for(int attempt=0;attempt<2;attempt++) {
      connect("shell");waitJS("location.pathname.endsWith('/terminal.html') && typeof term==='object' && active");
      js("term.paste('Android Shell 中文\\r');");
      waitJS("Array.from({length:term.buffer.active.length},(_,i)=>term.buffer.active.getLine(i)?.translateToString()||'').join('\\n').includes('回音：Android Shell 中文')");
      js("document.getElementById('back').click();");
      waitJS("location.pathname.endsWith('/index.html') && typeof openConnection==='function'");
      SystemClock.sleep(400);
    }
  }

  @Test public void rapidDirectReconnectReleasesPreviousSession() throws Exception {
    for(int attempt=0;attempt<8;attempt++) {
      Viewer session=new Viewer();
      try { session.connect("",host,secret); assertTrue(session.isConnected()); }
      finally { session.close(); }
    }
  }

  /** 真正的 TextureView 呈現回呼；不可只以收到封包或解碼器名稱判定成功。 */
  private AtomicInteger observeVideoFrames() throws Exception {
    AtomicInteger frames = new AtomicInteger();
    instrumentation.runOnMainSync(() -> {
      try {
        TextureView view = (TextureView) field("nativeVideo");
        TextureView.SurfaceTextureListener original = view.getSurfaceTextureListener();
        view.setSurfaceTextureListener(new TextureView.SurfaceTextureListener() {
          public void onSurfaceTextureAvailable(SurfaceTexture s, int w, int h) { original.onSurfaceTextureAvailable(s,w,h); }
          public void onSurfaceTextureSizeChanged(SurfaceTexture s, int w, int h) { original.onSurfaceTextureSizeChanged(s,w,h); }
          public boolean onSurfaceTextureDestroyed(SurfaceTexture s) { return original.onSurfaceTextureDestroyed(s); }
          public void onSurfaceTextureUpdated(SurfaceTexture s) { frames.incrementAndGet(); original.onSurfaceTextureUpdated(s); }
        });
      } catch (Exception e) { throw new AssertionError(e); }
    });
    return frames;
  }

  private String waitVideo(int codec, AtomicInteger frames, int after) throws Exception {
    AsyncVideoDecoder decoder = (AsyncVideoDecoder) field("videoDecoder");
    long deadline = SystemClock.uptimeMillis()+15000;
    while (SystemClock.uptimeMillis()<deadline) {
      if (frames.get()>after+4 && decoder.wireCodec()==codec && decoder.isStarted() && !decoder.needsKeyframe()) return decoder.backend();
      assertEquals("硬體影片不可在正常串流中被撤回；backend="+decoder.backend(),0,decoder.failedCodecMask());
      SystemClock.sleep(50);
    }
    throw new AssertionError("硬體影片沒有持續呈現；codec="+codec+" backend="+decoder.backend()+" frames="+frames.get());
  }

  private long uiHeartbeat() throws Exception {
    CountDownLatch done = new CountDownLatch(1);
    long begin = SystemClock.elapsedRealtime();
    new android.os.Handler(android.os.Looper.getMainLooper()).post(done::countDown);
    assertTrue("UI 被解碼或傳輸阻塞超過 1 秒",done.await(1,TimeUnit.SECONDS));
    return SystemClock.elapsedRealtime()-begin;
  }

  private String videoDiagnostic() throws Exception {
    AsyncVideoDecoder decoder=(AsyncVideoDecoder)field("videoDecoder");
    StringBuilder state=new StringBuilder("codec=").append(decoder.wireCodec())
        .append(" backend=").append(decoder.backend()).append(" started=").append(decoder.isStarted())
        .append(" needsKeyframe=").append(decoder.needsKeyframe()).append(" failed=").append(decoder.failedCodecMask());
    for(String name:new String[]{"generation","drops","active","acceptingDelta","resetRequested","recoveryOnly","rendered"}) {
      Field value=AsyncVideoDecoder.class.getDeclaredField(name);value.setAccessible(true);
      state.append(' ').append(name).append('=').append(value.get(decoder));
    }
    state.append(" foreground=").append(field("foreground")).append(" videoSequence=").append(field("videoSequence"));
    for(java.util.Map.Entry<Thread,StackTraceElement[]> thread:Thread.getAllStackTraces().entrySet()) {
      if(!thread.getKey().getName().equals("YourDesk-video-decoder"))continue;
      state.append(" worker=").append(java.util.Arrays.toString(thread.getValue()));
    }
    return state.toString();
  }

  private long decoderGeneration() throws Exception {
    AsyncVideoDecoder decoder=(AsyncVideoDecoder)field("videoDecoder");
    Field lock=AsyncVideoDecoder.class.getDeclaredField("lock"),generation=AsyncVideoDecoder.class.getDeclaredField("generation");
    lock.setAccessible(true);generation.setAccessible(true);
    synchronized(lock.get(decoder)) { return generation.getLong(decoder); }
  }

  @Test public void hardwareGopStreamingRemainsResponsiveAndRecovers() throws Exception {
    connect("desktop"); waitJS("location.pathname.endsWith('/desktop.html')");
    AtomicInteger frames = observeVideoFrames();
    int seconds = Math.max(10,Math.min(600,Integer.parseInt(InstrumentationRegistry.getArguments().getString("videoSoakSeconds","20"))));
    org.json.JSONArray results = new org.json.JSONArray();
    for (int codec : new int[]{1,2}) {
      String name=codec==1?"h264":"hevc",mime=codec==1?"video/avc":"video/hevc";
      assertNotNull("此驗收需要實際硬體解碼 "+mime,DecoderSupport.findHardwareDecoder(mime));
      int start=frames.get(); command("smoke.video."+name);
      String backend=waitVideo(codec,frames,start);
      long maxUi=0,maxControl=0,begin=SystemClock.elapsedRealtime();
      long lastProgress=begin;
      int steadyStart=frames.get();
      int last=frames.get(),idle=0;
      while(SystemClock.elapsedRealtime()-begin<seconds*1000L) {
        maxUi=Math.max(maxUi,uiHeartbeat());
        long sent=SystemClock.elapsedRealtime();
        // 走新版輸入工作者，Host 屏障只能在輸入真正抵達後回覆。
        command("smoke.input.clean");
        maxControl=Math.max(maxControl,SystemClock.elapsedRealtime()-sent);
        SystemClock.sleep(1000);
        int current=frames.get(); idle=current==last?idle+1:0; last=current;
        if(idle>0)android.util.Log.i("YourDeskDeviceSmoke","idle="+idle+" frames="+current+" "+videoDiagnostic());
        assertTrue("影像連續三秒沒有更新："+name+" "+(idle>=3?videoDiagnostic():""),idle<3);
        assertTrue("串流中控制回應超過 2 秒",maxControl<2000);
        assertEquals(0,((AsyncVideoDecoder)field("videoDecoder")).failedCodecMask());
        if(SystemClock.elapsedRealtime()-lastProgress>=30000) {
          lastProgress=SystemClock.elapsedRealtime();
          android.util.Log.i("YourDeskDeviceSmoke","progress codec="+name+" seconds="+((lastProgress-begin)/1000)
              +" rendered="+(current-steadyStart)+" maxUiMs="+maxUi+" maxControlMs="+maxControl);
        }
      }
      long elapsedMs=SystemClock.elapsedRealtime()-begin;
      int steadyRendered=frames.get()-steadyStart;
      assertTrue("持續串流必須有實際顯示影格",frames.get()-start>=seconds*15);
      long beforeDrop=decoderGeneration();command("smoke.video.drop");
      long recoveryDeadline=SystemClock.elapsedRealtime()+10000;
      while(decoderGeneration()==beforeDrop && SystemClock.elapsedRealtime()<recoveryDeadline)SystemClock.sleep(20);
      assertTrue("遺失影格應觸發實際恢復",decoderGeneration()>beforeDrop);
      waitVideo(codec,frames,frames.get());
      // 真正切換到背景，觸發 Surface 銷毀、延後釋放，再回前景重建。
      instrumentation.runOnMainSync(()->assertTrue(activity.moveTaskToBack(true)));
      waitField("foreground",false); SystemClock.sleep(600);
      int beforeResume=frames.get();
      instrumentation.runOnMainSync(()->activity.startActivity(new Intent(activity,MainActivity.class)
          .addFlags(Intent.FLAG_ACTIVITY_REORDER_TO_FRONT|Intent.FLAG_ACTIVITY_NEW_TASK)));
      waitField("foreground",true); waitVideo(codec,frames,beforeResume);
      JSONObject result=new JSONObject().put("codec",name).put("backend",backend).put("seconds",seconds)
          .put("rendered",frames.get()-start).put("steadyRendered",steadyRendered).put("elapsedMs",elapsedMs)
          .put("fps",steadyRendered*1000.0/elapsedMs).put("maxUiMs",maxUi).put("maxControlMs",maxControl);
      results.put(result); android.util.Log.i("YourDeskDeviceSmoke",result.toString());
      saveScreenshot("stream-"+name);
    }
    // 影片尚在輸出時操作正式關閉入口，再建立新連線。
    long closeBegin=SystemClock.elapsedRealtime();
    js("document.getElementById('back').click()");
    waitJS("location.pathname.endsWith('/index.html') && typeof openConnection==='function'");
    long closeMs=SystemClock.elapsedRealtime()-closeBegin;
    assertTrue("關閉影片工作階段不可等待驅動",closeMs<3000);
    connect("desktop"); waitJS("location.pathname.endsWith('/desktop.html')");
    int beforeReconnect=frames.get(); command("smoke.video.h264"); waitVideo(1,frames,beforeReconnect);
    JSONObject report=new JSONObject().put("device",android.os.Build.MODEL).put("android",android.os.Build.VERSION.RELEASE)
        .put("streams",results).put("closeMs",closeMs).put("reconnect",true);
    java.io.File output=new java.io.File(activity.getExternalFilesDir(null),"video-device-smoke.json");
    try(java.io.FileOutputStream file=new java.io.FileOutputStream(output)) {
      file.write(report.toString(2).getBytes(java.nio.charset.StandardCharsets.UTF_8));
    }
    android.util.Log.i("YourDeskDeviceSmoke",report.toString());
  }

  private void audioEnabled(boolean enabled) {
    audioAction("setRemoteAudioEnabled", boolean.class, enabled);
  }
  private void audioAction(String name, Class<?> type, Object value) {
    instrumentation.runOnMainSync(() -> {
      try { Method method=MainActivity.class.getDeclaredMethod(name,type);method.setAccessible(true);method.invoke(activity,value); }
      catch(Exception error){throw new AssertionError(error);}
    });
  }
  private RemoteAudioSession audio() throws Exception {return (RemoteAudioSession)field("remoteAudio");}
  private RemoteAudioSession.Stats waitAudio(String codec, long afterGeneration) throws Exception {
    long deadline=SystemClock.uptimeMillis()+18000;
    while(SystemClock.uptimeMillis()<deadline) {
      RemoteAudioSession.Stats stats=audio().stats();
      if(stats.generation>afterGeneration && stats.codec.equals(codec) && stats.playedFrames>2048 && stats.nonzeroSamples>1000)return stats;
      SystemClock.sleep(50);
    }
    fail("音訊未播放："+audio().status()+"；codec="+audio().stats().codec+"；frames="+audio().stats().playedFrames);
    return null;
  }
  private void waitAudioStopped() throws Exception {
    long deadline=SystemClock.uptimeMillis()+3000;
    while(audio().stats().generation!=0 && SystemClock.uptimeMillis()<deadline)SystemClock.sleep(20);
    assertEquals("已釋放本機音訊解碼及播放裝置",0,audio().stats().generation);
  }
  private void tapAudio() {
    instrumentation.runOnMainSync(() -> {
      try {assertTrue(((android.widget.Button)field("audioButton")).performClick());}
      catch(Exception error){throw new AssertionError(error);}
    });
  }

  @Test public void audioNegotiatesOpusAacPcmAndMute() throws Exception {
    connect("desktop");waitJS("location.pathname.endsWith('/desktop.html')");
    assertEquals("預設不開啟聲音",0,audio().stats().generation);
    long previous=0;
    for(String codec:new String[]{"opus","aac","pcm"}) {
      ((Viewer)field("viewer")).callCommand("smoke.audio."+codec);
      tapAudio();
      RemoteAudioSession.Stats stats=waitAudio(codec.equals("aac")?"aac-software":codec,previous);
      previous=stats.generation;
      ((Viewer)field("viewer")).callCommand("ping");
      assertTrue("聲音不能阻塞桌面影像",Color.blue(centerPixel())>150);
      tapAudio();waitAudioStopped();
    }
  }

  @Test public void audioFallbackKeepsDesktopConnected() throws Exception {
    connect("desktop");waitJS("location.pathname.endsWith('/desktop.html')");
    ((Viewer)field("viewer")).callCommand("smoke.audio.fallback");
    tapAudio();waitAudio("aac-software",0);
    assertTrue("來源 Opus 失敗後仍保持桌面連線",((Viewer)field("viewer")).isConnected());
    ((Viewer)field("viewer")).callCommand("ping");
    tapAudio();waitAudioStopped();
  }

  @Test public void unavailableAudioReleasesFocusAndCanRetry() throws Exception {
    connect("desktop");waitJS("location.pathname.endsWith('/desktop.html')");
    ((Viewer)field("viewer")).callCommand("smoke.audio.none");
    tapAudio();
    long deadline=SystemClock.uptimeMillis()+10000;
    while(!audio().unavailable() && SystemClock.uptimeMillis()<deadline)SystemClock.sleep(50);
    assertTrue("無共同格式時明確回報",audio().status().contains("沒有共同可用"));
    instrumentation.waitForIdleSync();
    assertEquals("無法播放時釋放音訊焦點",false,field("audioFocusGranted"));
    waitAudioStopped(); ((Viewer)field("viewer")).callCommand("ping");
    assertTrue("聲音失敗不影響畫面",Color.blue(centerPixel())>150);
    tapAudio(); ((Viewer)field("viewer")).callCommand("smoke.audio.opus");
    tapAudio(); waitAudio("opus",0);
  }

  @Test public void audioStopsOnFocusBackgroundDisconnectAndReconnects() throws Exception {
    connect("desktop");waitJS("location.pathname.endsWith('/desktop.html')");
    ((Viewer)field("viewer")).callCommand("smoke.audio.opus");
    tapAudio();long previous=waitAudio("opus",0).generation;
    audioAction("onAudioFocusChanged",int.class,android.media.AudioManager.AUDIOFOCUS_LOSS_TRANSIENT);
    waitAudioStopped();
    audioAction("onAudioFocusChanged",int.class,android.media.AudioManager.AUDIOFOCUS_GAIN);
    previous=waitAudio("opus",previous).generation;
    instrumentation.runOnMainSync(() -> assertTrue(activity.moveTaskToBack(true)));
    waitAudioStopped();
    instrumentation.runOnMainSync(() -> activity.startActivity(new Intent(activity,MainActivity.class)
        .addFlags(Intent.FLAG_ACTIVITY_REORDER_TO_FRONT | Intent.FLAG_ACTIVITY_NEW_TASK)));
    previous=waitAudio("opus",previous).generation;
    instrumentation.runOnMainSync(() -> {
      try { ((android.content.BroadcastReceiver)field("audioNoisyReceiver")).onReceive(activity,
          new Intent(android.media.AudioManager.ACTION_AUDIO_BECOMING_NOISY)); }
      catch(Exception error){throw new AssertionError(error);}
    });
    waitAudioStopped(); assertEquals(false,field("audioEnabled"));
    tapAudio(); previous=waitAudio("opus",previous).generation;
    ((Viewer)field("viewer")).callCommand("session.disconnect");
    waitJS("location.pathname.endsWith('/index.html') && document.body.innerText.includes('遠端已斷線')");
    waitAudioStopped();
    connect("desktop");waitJS("location.pathname.endsWith('/desktop.html')");
    waitAudio("opus",previous);
  }
}
