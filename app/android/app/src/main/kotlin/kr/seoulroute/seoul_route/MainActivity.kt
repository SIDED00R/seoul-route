package kr.seoulroute.seoul_route

import android.Manifest
import android.content.pm.PackageManager
import android.os.Build
import io.flutter.embedding.android.FlutterActivity
import io.flutter.embedding.engine.FlutterEngine
import io.flutter.plugin.common.MethodChannel

// 안내 알림(위치 포그라운드 서비스)을 알림창에 띄우는 POST_NOTIFICATIONS 요청 채널(lib/guide/background_location.dart).
// Android 13(API 33) 미만은 이 런타임 권한이 없어 바로 true 를 돌려준다.
class MainActivity : FlutterActivity() {
    private var pending: MethodChannel.Result? = null

    override fun configureFlutterEngine(flutterEngine: FlutterEngine) {
        super.configureFlutterEngine(flutterEngine)
        MethodChannel(flutterEngine.dartExecutor.binaryMessenger, CHANNEL).setMethodCallHandler { call, result ->
            when {
                call.method != "request" -> result.notImplemented()
                Build.VERSION.SDK_INT < Build.VERSION_CODES.TIRAMISU ||
                    checkSelfPermission(Manifest.permission.POST_NOTIFICATIONS) == PackageManager.PERMISSION_GRANTED ->
                    result.success(true)
                pending != null -> result.success(false)
                else -> {
                    pending = result
                    requestPermissions(arrayOf(Manifest.permission.POST_NOTIFICATIONS), REQUEST_CODE)
                }
            }
        }
    }

    override fun onRequestPermissionsResult(requestCode: Int, permissions: Array<String>, grantResults: IntArray) {
        super.onRequestPermissionsResult(requestCode, permissions, grantResults)
        if (requestCode != REQUEST_CODE) return
        pending?.success(grantResults.firstOrNull() == PackageManager.PERMISSION_GRANTED)
        pending = null
    }

    companion object {
        private const val CHANNEL = "seoul_route/notification_permission"
        private const val REQUEST_CODE = 7301
    }
}
