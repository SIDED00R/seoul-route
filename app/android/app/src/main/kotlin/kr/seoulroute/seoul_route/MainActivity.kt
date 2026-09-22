package kr.seoulroute.seoul_route

import android.Manifest
import android.content.pm.PackageManager
import android.content.Intent
import android.net.Uri
import android.os.Build
import android.provider.Settings
import io.flutter.embedding.android.FlutterActivity
import io.flutter.embedding.engine.FlutterEngine
import io.flutter.plugin.common.MethodChannel

// 안내 알림(위치 포그라운드 서비스)을 알림창에 띄우는 POST_NOTIFICATIONS 요청 채널(lib/guide/background_location.dart).
// Android 13(API 33) 미만은 이 런타임 권한이 없어 바로 true 를 돌려준다.
class MainActivity : FlutterActivity() {
    private var pendingNotification: MethodChannel.Result? = null
    private var pendingOverlayPermission: MethodChannel.Result? = null
    private lateinit var guideOverlay: GuideOverlayController

    override fun configureFlutterEngine(flutterEngine: FlutterEngine) {
        super.configureFlutterEngine(flutterEngine)
        guideOverlay = GuideOverlayController(applicationContext)
        // 안내 진행 알림(lib/guide/status_notification.dart). 화면이 닫힐 때 앱이 cancel 을 부른다.
        val status = GuideStatusNotification(applicationContext)
        MethodChannel(flutterEngine.dartExecutor.binaryMessenger, STATUS_CHANNEL).setMethodCallHandler { call, result ->
            when (call.method) {
                "show" -> {
                    status.show(call.argument<String>("title") ?: "", call.argument<String>("text") ?: "")
                    result.success(null)
                }
                "cancel" -> {
                    status.cancel()
                    result.success(null)
                }
                else -> result.notImplemented()
            }
        }
        // 안내 중 뒤로가기(lib/util/app_task.dart). 액티비티를 끝내면 안내가 통째로 사라지므로 뒤로 보내기만 한다.
        MethodChannel(flutterEngine.dartExecutor.binaryMessenger, TASK_CHANNEL).setMethodCallHandler { call, result ->
            when (call.method) {
                "moveToBack" -> result.success(moveTaskToBack(true))
                else -> result.notImplemented()
            }
        }
        val overlayChannel = MethodChannel(flutterEngine.dartExecutor.binaryMessenger, OVERLAY_CHANNEL)
        // 미니 지도 손잡이 슬라이더에서 손을 뗀 값을 앱에 알려 설정(overlay_opacity)에 저장하게 한다.
        guideOverlay.onOpacityCommitted = { value -> overlayChannel.invokeMethod("opacityChanged", value.toDouble()) }
        overlayChannel.setMethodCallHandler { call, result ->
            when (call.method) {
                "requestPermission" -> requestOverlayPermission(result)
                "setEnabled" -> {
                    val enabled = call.argument<Boolean>("enabled") == true
                    call.argument<Double>("opacity")?.let {
                        guideOverlay.setOpacity(it.toFloat())
                    }
                    guideOverlay.setEnabled(enabled)
                    result.success(null)
                }
                "setOpacity" -> {
                    val opacity = call.argument<Double>("opacity")
                    if (opacity == null) result.error("bad_args", "opacity 가 없습니다", null)
                    else {
                        guideOverlay.setOpacity(opacity.toFloat())
                        result.success(null)
                    }
                }
                "update" -> {
                    @Suppress("UNCHECKED_CAST")
                    val data = call.arguments as? Map<String, Any?>
                    if (data == null) result.error("bad_args", "오버레이 데이터가 없습니다", null)
                    else {
                        guideOverlay.update(data)
                        result.success(null)
                    }
                }
                else -> result.notImplemented()
            }
        }
        MethodChannel(flutterEngine.dartExecutor.binaryMessenger, CHANNEL).setMethodCallHandler { call, result ->
            when {
                call.method != "request" -> result.notImplemented()
                Build.VERSION.SDK_INT < Build.VERSION_CODES.TIRAMISU ||
                    checkSelfPermission(Manifest.permission.POST_NOTIFICATIONS) == PackageManager.PERMISSION_GRANTED ->
                    result.success(true)
                pendingNotification != null -> result.success(false)
                else -> {
                    pendingNotification = result
                    requestPermissions(arrayOf(Manifest.permission.POST_NOTIFICATIONS), REQUEST_CODE)
                }
            }
        }
    }

    override fun onRequestPermissionsResult(requestCode: Int, permissions: Array<String>, grantResults: IntArray) {
        super.onRequestPermissionsResult(requestCode, permissions, grantResults)
        if (requestCode != REQUEST_CODE) return
        pendingNotification?.success(grantResults.firstOrNull() == PackageManager.PERMISSION_GRANTED)
        pendingNotification = null
    }

    private fun requestOverlayPermission(result: MethodChannel.Result) {
        if (Settings.canDrawOverlays(this)) {
            result.success(true)
            return
        }
        if (pendingOverlayPermission != null) {
            result.success(false)
            return
        }
        pendingOverlayPermission = result
        try {
            startActivityForResult(
                Intent(Settings.ACTION_MANAGE_OVERLAY_PERMISSION, Uri.parse("package:$packageName")),
                OVERLAY_REQUEST_CODE,
            )
        } catch (_: Exception) {
            pendingOverlayPermission = null
            result.success(false)
        }
    }

    @Deprecated("Deprecated in Android")
    override fun onActivityResult(requestCode: Int, resultCode: Int, data: Intent?) {
        super.onActivityResult(requestCode, resultCode, data)
        if (requestCode != OVERLAY_REQUEST_CODE) return
        pendingOverlayPermission?.success(Settings.canDrawOverlays(this))
        pendingOverlayPermission = null
    }

    override fun onUserLeaveHint() {
        super.onUserLeaveHint()
        if (::guideOverlay.isInitialized) guideOverlay.onAppBackgrounded()
    }

    override fun onResume() {
        super.onResume()
        // 일부 제조사 설정 화면은 onActivityResult 를 보내지 않는다. 앱으로 돌아온 시점에도 권한 요청을 끝낸다.
        pendingOverlayPermission?.success(Settings.canDrawOverlays(this))
        pendingOverlayPermission = null
        if (::guideOverlay.isInitialized) guideOverlay.onAppForegrounded()
    }

    override fun onDestroy() {
        // isFinishing 이 아닌 파괴("활동 보존 안 함"·백그라운드 회수)에서도 정리한다. 이 Activity 의 Flutter 엔진(안내 세션)이
        // 같이 죽어 창을 갱신할 주체가 없고, 재생성된 Activity 가 새 창을 띄우면 두 겹이 된다(진하기가 높으면 합산
        // 불투명도가 터치 상한을 넘겨 뒤 앱 터치도 막힌다 — GuideOverlayController 클래스 주석의 합산 규칙).
        if (::guideOverlay.isInitialized) guideOverlay.stop()
        super.onDestroy()
    }

    companion object {
        private const val CHANNEL = "seoul_route/notification_permission"
        private const val STATUS_CHANNEL = "seoul_route/guide_status"
        private const val TASK_CHANNEL = "seoul_route/app_task"
        private const val OVERLAY_CHANNEL = "seoul_route/guide_overlay"
        private const val REQUEST_CODE = 7301
        private const val OVERLAY_REQUEST_CODE = 7303
    }
}
