package kr.seoulroute.seoul_route

import android.content.Context
import android.media.AudioAttributes
import android.media.AudioFocusRequest
import android.media.AudioManager
import android.os.Build

// 안내 음성이 나오는 동안 다른 앱(영상·음악) 소리를 줄이는 오디오 포커스(lib/guide/audio_focus.dart). 요청 객체 하나를
// 잡고 놓는다 — duck 을 여러 번 불러도 한 번만 잡고, release 는 잡은 것을 놓는다. flutter_tts 의 speak(focus: true) 는
// 쓰지 않는다(발화마다 새 요청을 만들어 앞 요청을 놓지 않는다).
class GuideAudioFocus(context: Context) {
    private val audioManager = context.getSystemService(Context.AUDIO_SERVICE) as AudioManager
    private val request: AudioFocusRequest? =
        if (Build.VERSION.SDK_INT >= Build.VERSION_CODES.O) {
            AudioFocusRequest.Builder(AudioManager.AUDIOFOCUS_GAIN_TRANSIENT_MAY_DUCK)
                .setAudioAttributes(
                    AudioAttributes.Builder()
                        .setUsage(AudioAttributes.USAGE_ASSISTANCE_NAVIGATION_GUIDANCE)
                        .setContentType(AudioAttributes.CONTENT_TYPE_SPEECH)
                        .build(),
                )
                .setOnAudioFocusChangeListener { }
                .build()
        } else {
            null
        }
    private var held = false

    fun duck() {
        if (held) return
        held = true
        if (request != null) {
            audioManager.requestAudioFocus(request)
        } else {
            @Suppress("DEPRECATION")
            audioManager.requestAudioFocus(
                null, AudioManager.STREAM_MUSIC, AudioManager.AUDIOFOCUS_GAIN_TRANSIENT_MAY_DUCK,
            )
        }
    }

    fun release() {
        if (!held) return
        held = false
        if (request != null) {
            audioManager.abandonAudioFocusRequest(request)
        } else {
            @Suppress("DEPRECATION")
            audioManager.abandonAudioFocus(null)
        }
    }
}
