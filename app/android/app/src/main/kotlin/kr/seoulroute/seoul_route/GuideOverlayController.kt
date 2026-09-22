package kr.seoulroute.seoul_route

import android.content.Context
import android.graphics.PixelFormat
import android.hardware.input.InputManager
import android.os.Build
import android.util.Log
import android.view.Gravity
import android.view.WindowManager
import io.flutter.FlutterInjector
import io.flutter.embedding.android.FlutterTextureView
import io.flutter.embedding.android.FlutterView
import io.flutter.embedding.engine.FlutterEngine
import io.flutter.embedding.engine.dart.DartExecutor
import io.flutter.plugin.common.MethodChannel

/**
 * 안내 중 앱을 떠날 때만 보이는 터치 통과 미니 지도. 별도 Flutter 엔진이 overlayMain 을 그린다.
 * 창은 TextureView 로 그린다 — SurfaceView 면 창과 서피스 층이 각각 "가리는 창" 으로 합산돼(alpha 0.7 이면 1-0.3²=0.91)
 * Android 12+ 의 터치 상한 0.8 을 넘겨 뒤 앱 터치가 버려진다(실기기 InputDispatcher "Untrusted touch" 실측).
 */
class GuideOverlayController(private val appContext: Context) {
    private var enabled = false
    private var opacity = DEFAULT_OPACITY
    private var engine: FlutterEngine? = null
    private var view: FlutterView? = null
    private var channel: MethodChannel? = null
    private var windowManager: WindowManager? = null
    private var showing = false
    private var appInBackground = false
    private var latest: Map<String, Any?>? = null
    private var controls: GuideOverlayControls? = null // 구석 손잡이·슬라이더 띠(터치를 받는 유일한 창)

    /** 사용자가 손잡이 슬라이더에서 손을 뗐을 때의 값. MainActivity 가 앱(Dart)에 넘겨 설정에 저장한다. */
    var onOpacityCommitted: ((Float) -> Unit)? = null

    fun setEnabled(value: Boolean) {
        enabled = value
        if (!value) {
            hide()
            destroyEngine()
        } else if (android.provider.Settings.canDrawOverlays(appContext)) {
            prepareEngine()
            if (appInBackground) show()
        }
    }

    /** 창 불투명도. 떠 있으면 바로 다시 그린다. 값은 기기 터치 상한으로 자른다(touchSafeAlpha). */
    fun setOpacity(value: Float) {
        opacity = value.coerceIn(0.05f, 1f)
        controls?.setOpacity(opacity)
        val flutterView = view ?: return
        val wm = windowManager ?: return
        if (!showing) return
        val params = flutterView.layoutParams as? WindowManager.LayoutParams ?: return
        params.alpha = touchSafeAlpha()
        try {
            wm.updateViewLayout(flutterView, params)
        } catch (_: IllegalArgumentException) {
            // 창이 이미 제거됐다.
        }
    }

    /** Android 12+ 는 다른 앱을 가리는 non-touchable 창의 불투명도가 상한(보통 0.8)을 넘으면 뒤 앱 터치를 막는다. */
    private fun touchSafeAlpha(): Float {
        val maxAlpha = if (Build.VERSION.SDK_INT >= Build.VERSION_CODES.S) {
            val input = appContext.getSystemService(Context.INPUT_SERVICE) as InputManager
            input.maximumObscuringOpacityForTouch
        } else {
            0.8f
        }
        return minOf(opacity, maxAlpha)
    }

    fun update(data: Map<String, Any?>) {
        latest = data
        if (enabled && android.provider.Settings.canDrawOverlays(appContext)) prepareEngine()
        if (showing) sendLatest()
        else if (appInBackground) show()
    }

    fun onAppBackgrounded() {
        appInBackground = true
        show()
    }

    fun onAppForegrounded() {
        appInBackground = false
        hide()
    }

    fun show() {
        if (!enabled || latest == null || !android.provider.Settings.canDrawOverlays(appContext) || showing) return
        if (!prepareEngine()) return
        val flutterView = view ?: return
        val wm = appContext.getSystemService(Context.WINDOW_SERVICE) as WindowManager
        val params = WindowManager.LayoutParams(
            WindowManager.LayoutParams.MATCH_PARENT,
            dp(OVERLAY_HEIGHT_DP),
            if (Build.VERSION.SDK_INT >= Build.VERSION_CODES.O) {
                WindowManager.LayoutParams.TYPE_APPLICATION_OVERLAY
            } else {
                @Suppress("DEPRECATION")
                WindowManager.LayoutParams.TYPE_PHONE
            },
            WindowManager.LayoutParams.FLAG_NOT_FOCUSABLE or
                WindowManager.LayoutParams.FLAG_NOT_TOUCHABLE or
                WindowManager.LayoutParams.FLAG_LAYOUT_IN_SCREEN,
            PixelFormat.TRANSLUCENT,
        ).apply {
            gravity = Gravity.TOP or Gravity.START
            alpha = touchSafeAlpha()
        }
        try {
            wm.addView(flutterView, params)
            windowManager = wm
            showing = true
            sendLatest()
            controls = GuideOverlayControls(
                appContext,
                onPreview = { setOpacity(it) },
                onCommit = {
                    setOpacity(it)
                    onOpacityCommitted?.invoke(it)
                },
            ).also { it.show(opacity) }
        } catch (e: WindowManager.BadTokenException) {
            Log.w(TAG, "window token rejected", e)
            showing = false
        } catch (e: SecurityException) {
            Log.w(TAG, "overlay permission rejected", e)
            showing = false
        }
    }

    fun hide() {
        if (!showing) return
        controls?.hide()
        controls = null
        val flutterView = view
        val flutterEngine = engine
        if (flutterView != null && flutterEngine != null) {
            flutterEngine.lifecycleChannel.appIsPaused()
            flutterView.detachFromFlutterEngine()
        }
        try {
            flutterView?.let { windowManager?.removeView(it) }
        } catch (_: IllegalArgumentException) {
            // 시스템이 이미 창을 제거했다.
        } finally {
            showing = false
            windowManager = null
            // 창 밖에서 계속 도는 FlutterView 는 접근성 이벤트를 부모 없는 뷰로 보내므로 함께 폐기한다.
            view = null
        }
        destroyEngine()
    }

    fun stop() {
        enabled = false
        latest = null
        hide()
        destroyEngine()
    }

    private fun prepareEngine(): Boolean {
        if (engine != null && view != null) return true
        destroyEngine()
        var candidateEngine: FlutterEngine? = null
        return try {
            val loader = FlutterInjector.instance().flutterLoader()
            loader.startInitialization(appContext)
            loader.ensureInitializationComplete(appContext, null)
            val newEngine = FlutterEngine(appContext)
            candidateEngine = newEngine
            val dataChannel = MethodChannel(newEngine.dartExecutor.binaryMessenger, DATA_CHANNEL)
            dataChannel.setMethodCallHandler { call, result ->
                if (call.method == "ready") {
                    result.success(null)
                    sendLatest()
                } else {
                    result.notImplemented()
                }
            }
            val entrypoint = DartExecutor.DartEntrypoint(loader.findAppBundlePath(), "overlayMain")
            newEngine.dartExecutor.executeDartEntrypoint(entrypoint)
            newEngine.lifecycleChannel.appIsResumed()
            val newView = FlutterView(appContext, FlutterTextureView(appContext)) // 창 한 겹만 가리게(클래스 주석)
            newView.attachToFlutterEngine(newEngine)
            engine = newEngine
            channel = dataChannel
            view = newView
            candidateEngine = null
            true
        } catch (e: RuntimeException) {
            Log.e(TAG, "engine preparation failed", e)
            candidateEngine?.destroy()
            destroyEngine()
            false
        }
    }

    private fun sendLatest() {
        latest?.let { channel?.invokeMethod("update", it) }
    }

    private fun destroyEngine() {
        val flutterView = view
        val flutterEngine = engine
        if (flutterView != null && flutterEngine != null) flutterView.detachFromFlutterEngine()
        channel?.setMethodCallHandler(null)
        flutterEngine?.destroy()
        view = null
        channel = null
        engine = null
    }

    private fun dp(value: Int): Int = (value * appContext.resources.displayMetrics.density).toInt()

    companion object {
        const val DATA_CHANNEL = "seoul_route/guide_overlay_data"
        private const val TAG = "GuideOverlay"
        private const val OVERLAY_HEIGHT_DP = 260 // 미니 지도 창 높이. 손잡이 창(GuideOverlayControls)은 이 창 안쪽 오른쪽 위에 따로 뜬다
        private const val DEFAULT_OPACITY = 0.5f // 앱이 setEnabled 로 설정값을 넘기기 전 기본(Settings.defaultOverlayOpacity 와 같다)
    }
}
