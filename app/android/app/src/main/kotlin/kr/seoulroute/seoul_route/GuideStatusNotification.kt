package kr.seoulroute.seoul_route

import android.app.Notification
import android.app.NotificationChannel
import android.app.NotificationManager
import android.app.PendingIntent
import android.content.Context
import android.content.Intent
import android.os.Build

// 알림창의 안내 진행 알림(지금 할 일·도착 예정·남은 시간). lib/guide/status_notification.dart 가 내용을 보낸다.
// 위치 포그라운드 서비스의 "안내 중" 알림은 geolocator 가 띄우며 문구를 바꿀 수 없어서 따로 둔다.
class GuideStatusNotification(private val context: Context) {
    private val manager = context.getSystemService(Context.NOTIFICATION_SERVICE) as NotificationManager

    fun show(title: String, text: String) {
        if (Build.VERSION.SDK_INT >= Build.VERSION_CODES.O) {
            // IMPORTANCE_LOW: 소리·진동·헤드업 없이 알림창에만 보인다(내용이 몇 초마다 바뀐다).
            manager.createNotificationChannel(
                NotificationChannel(CHANNEL_ID, "안내 진행", NotificationManager.IMPORTANCE_LOW)
            )
        }
        val open = PendingIntent.getActivity(
            context, 0,
            Intent(context, MainActivity::class.java).addFlags(Intent.FLAG_ACTIVITY_SINGLE_TOP),
            PendingIntent.FLAG_UPDATE_CURRENT or PendingIntent.FLAG_IMMUTABLE
        )
        val builder = if (Build.VERSION.SDK_INT >= Build.VERSION_CODES.O) {
            Notification.Builder(context, CHANNEL_ID)
        } else {
            @Suppress("DEPRECATION")
            Notification.Builder(context)
        }
        val notification = builder
            .setSmallIcon(android.R.drawable.ic_menu_directions)
            .setContentTitle(title)
            .setContentText(text)
            .setStyle(Notification.BigTextStyle().bigText(text))
            .setContentIntent(open)
            .setOngoing(true)
            .setOnlyAlertOnce(true)
            .setShowWhen(false)
            .build()
        manager.notify(NOTIFICATION_ID, notification)
    }

    fun cancel() = manager.cancel(NOTIFICATION_ID)

    companion object {
        private const val CHANNEL_ID = "guide_status"
        private const val NOTIFICATION_ID = 7302
    }
}
