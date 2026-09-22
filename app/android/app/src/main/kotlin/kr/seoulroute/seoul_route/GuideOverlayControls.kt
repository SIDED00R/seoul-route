package kr.seoulroute.seoul_route

import android.content.Context
import android.content.res.ColorStateList
import android.graphics.Color
import android.graphics.PixelFormat
import android.graphics.drawable.GradientDrawable
import android.os.Build
import android.os.Handler
import android.os.Looper
import android.util.Log
import android.util.TypedValue
import android.view.Gravity
import android.view.MotionEvent
import android.view.View
import android.view.WindowManager
import android.widget.FrameLayout
import android.widget.ImageView
import android.widget.LinearLayout
import android.widget.SeekBar
import android.widget.TextView
import kotlin.math.roundToInt

/**
 * 미니 지도(터치 통과 창) 구석의 손잡이(⚙)와, 탭하면 그 자리에 펼쳐지는 진하기 슬라이더 띠.
 * 미니 지도 창은 터치를 못 받으므로 이 작은 창이 대신 입력을 받는다 — 손잡이 44dp(펼치면 띠 56dp 한 줄) 밖은
 * FLAG_NOT_TOUCH_MODAL 로 뒤 앱에 그대로 간다. 끄는 동안 [onPreview] 로 미니 지도에 바로 반영하고, 손을 떼면
 * [onCommit] 으로 설정에 저장한다. 바깥을 누르거나(FLAG_WATCH_OUTSIDE_TOUCH) 4초 동안 입력이 없으면 손잡이로 접힌다.
 */
class GuideOverlayControls(
    private val appContext: Context,
    private val overlayHeightPx: Int,
    private val onPreview: (Float) -> Unit,
    private val onCommit: (Float) -> Unit,
) {
    private var windowManager: WindowManager? = null
    private var root: View? = null
    private var label: TextView? = null
    private var expanded = false
    private var opacity = 0.5f
    private val handler = Handler(Looper.getMainLooper())
    private val collapse = Runnable { if (expanded) show(expanded = false) }

    /** 손잡이 상태로 띄운다(미니 지도가 뜰 때). */
    fun show(opacity: Float) {
        this.opacity = opacity
        show(expanded = false)
    }

    /** 앱 쪽에서 값이 바뀌었을 때 띠의 표시만 맞춘다. */
    fun setOpacity(value: Float) {
        opacity = value
        label?.text = percent(value)
    }

    fun hide() {
        handler.removeCallbacks(collapse)
        root?.let { view ->
            try {
                windowManager?.removeView(view)
            } catch (_: IllegalArgumentException) {
                // 시스템이 이미 창을 제거했다.
            }
        }
        root = null
        label = null
        windowManager = null
        expanded = false
    }

    private fun show(expanded: Boolean) {
        val wm = windowManager
            ?: (appContext.getSystemService(Context.WINDOW_SERVICE) as WindowManager).also { windowManager = it }
        root?.let { old ->
            try {
                wm.removeView(old)
            } catch (_: IllegalArgumentException) {
                // 이미 제거됨
            }
        }
        root = null
        label = null
        this.expanded = expanded
        val view = if (expanded) buildStrip() else buildHandle()
        try {
            wm.addView(view, params(expanded))
            root = view
        } catch (e: WindowManager.BadTokenException) {
            Log.w(TAG, "controls window rejected", e)
        } catch (e: SecurityException) {
            Log.w(TAG, "controls window permission rejected", e)
        }
        handler.removeCallbacks(collapse)
        if (expanded) handler.postDelayed(collapse, COLLAPSE_MS)
    }

    private fun params(expanded: Boolean): WindowManager.LayoutParams {
        val height = dp(if (expanded) STRIP_DP else HANDLE_DP)
        val width = if (expanded) WindowManager.LayoutParams.MATCH_PARENT else dp(HANDLE_DP)
        return WindowManager.LayoutParams(
            width,
            height,
            if (Build.VERSION.SDK_INT >= Build.VERSION_CODES.O) {
                WindowManager.LayoutParams.TYPE_APPLICATION_OVERLAY
            } else {
                @Suppress("DEPRECATION")
                WindowManager.LayoutParams.TYPE_PHONE
            },
            WindowManager.LayoutParams.FLAG_NOT_FOCUSABLE or
                WindowManager.LayoutParams.FLAG_NOT_TOUCH_MODAL or
                WindowManager.LayoutParams.FLAG_LAYOUT_IN_SCREEN or
                WindowManager.LayoutParams.FLAG_WATCH_OUTSIDE_TOUCH,
            PixelFormat.TRANSLUCENT,
        ).apply {
            gravity = Gravity.TOP or Gravity.END
            x = if (expanded) 0 else dp(MARGIN_DP)
            y = overlayHeightPx - height - dp(MARGIN_DP) // 미니 지도 창의 아래쪽 가장자리 안
        }
    }

    private fun buildHandle(): View {
        val icon = ImageView(appContext).apply {
            setImageResource(android.R.drawable.ic_menu_preferences)
            imageTintList = ColorStateList.valueOf(Color.WHITE)
            val pad = dp(10)
            setPadding(pad, pad, pad, pad)
        }
        return FrameLayout(appContext).apply {
            background = GradientDrawable().apply {
                shape = GradientDrawable.OVAL
                setColor(0xB3000000.toInt())
            }
            contentDescription = "미니 지도 진하기"
            addView(
                icon,
                FrameLayout.LayoutParams(FrameLayout.LayoutParams.MATCH_PARENT, FrameLayout.LayoutParams.MATCH_PARENT),
            )
            setOnClickListener { show(expanded = true) }
        }
    }

    private fun buildStrip(): View {
        val title = TextView(appContext).apply {
            text = "진하기"
            setTextColor(Color.WHITE)
            setTextSize(TypedValue.COMPLEX_UNIT_SP, 13f)
        }
        val value = TextView(appContext).apply {
            text = percent(opacity)
            setTextColor(Color.WHITE)
            setTextSize(TypedValue.COMPLEX_UNIT_SP, 13f)
            minWidth = dp(44)
            gravity = Gravity.END
        }
        label = value
        val seek = SeekBar(appContext).apply {
            max = MAX_PERCENT - MIN_PERCENT
            progress = (opacity * 100).roundToInt() - MIN_PERCENT
            progressTintList = ColorStateList.valueOf(Color.WHITE)
            thumbTintList = ColorStateList.valueOf(Color.WHITE)
            setOnSeekBarChangeListener(object : SeekBar.OnSeekBarChangeListener {
                override fun onProgressChanged(bar: SeekBar, progress: Int, fromUser: Boolean) {
                    if (!fromUser) return
                    opacity = (progress + MIN_PERCENT) / 100f
                    value.text = percent(opacity)
                    onPreview(opacity)
                    restartCollapse()
                }

                override fun onStartTrackingTouch(bar: SeekBar) {
                    handler.removeCallbacks(collapse)
                }

                override fun onStopTrackingTouch(bar: SeekBar) {
                    onCommit(opacity)
                    restartCollapse()
                }
            })
        }
        return LinearLayout(appContext).apply {
            orientation = LinearLayout.HORIZONTAL
            gravity = Gravity.CENTER_VERTICAL
            background = GradientDrawable().apply {
                cornerRadius = dp(12).toFloat()
                setColor(0xE6000000.toInt()) // 아래 안내 문구가 비쳐 보이지 않게 거의 불투명

            }
            setPadding(dp(12), dp(4), dp(12), dp(4))
            addView(title)
            addView(seek, LinearLayout.LayoutParams(0, LinearLayout.LayoutParams.WRAP_CONTENT, 1f).apply {
                marginStart = dp(8)
                marginEnd = dp(8)
            })
            addView(value)
            // 띠 밖을 누르면 접는다. 이벤트를 소비하지 않아 뒤 앱의 터치는 그대로 간다.
            setOnTouchListener { _, event ->
                if (event.action == MotionEvent.ACTION_OUTSIDE) show(expanded = false)
                false
            }
        }
    }

    private fun restartCollapse() {
        handler.removeCallbacks(collapse)
        handler.postDelayed(collapse, COLLAPSE_MS)
    }

    private fun percent(value: Float): String = "${(value * 100).roundToInt()}%"

    private fun dp(value: Int): Int = (value * appContext.resources.displayMetrics.density).toInt()

    companion object {
        private const val TAG = "GuideOverlayControls"
        private const val HANDLE_DP = 44
        private const val STRIP_DP = 56
        private const val MARGIN_DP = 8
        private const val COLLAPSE_MS = 4000L
        // 앱 설정 슬라이더와 같은 범위(Settings.minOverlayOpacity / maxOverlayOpacity). 80 은 Android 터치 차단 상한.
        private const val MIN_PERCENT = 20
        private const val MAX_PERCENT = 80
    }
}
