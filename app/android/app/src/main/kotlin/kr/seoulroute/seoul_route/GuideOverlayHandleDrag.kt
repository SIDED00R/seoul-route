package kr.seoulroute.seoul_route

import android.os.Handler
import android.os.Looper
import android.view.HapticFeedbackConstants
import android.view.MotionEvent
import android.view.View
import android.view.ViewConfiguration
import kotlin.math.abs
import kotlin.math.roundToInt

/**
 * 진하기 손잡이의 손짓 판정(안드로이드 의존 없음). 짧게 눌렀다 떼면 TAP, 길게 누르면 DRAG_START 뒤 움직임마다 DRAG,
 * 떼면 DROP. 길게 누르기 전에 slop 보다 많이 움직이면 그 손짓은 아무것도 아니다(뒤 앱을 스크롤하려던 것일 수 있다).
 */
class HandleGesture(private val slop: Float) {
    enum class Action { NONE, TAP, DRAG_START, DRAG, DROP }

    private var downX = 0f
    private var downY = 0f
    var moved = false
        private set
    var dragging = false
        private set

    /** 누른 곳에서 지금까지 움직인 양(px). DRAG 일 때 쓴다. */
    var dx = 0
        private set
    var dy = 0
        private set

    fun down(x: Float, y: Float) {
        downX = x
        downY = y
        moved = false
        dragging = false
        dx = 0
        dy = 0
    }

    /** 길게 누르기 시간이 지났을 때. 그 사이 움직이지 않았으면 끌기를 시작한다. */
    fun longPress(): Action {
        if (moved || dragging) return Action.NONE
        dragging = true
        return Action.DRAG_START
    }

    fun move(x: Float, y: Float): Action {
        dx = (x - downX).roundToInt()
        dy = (y - downY).roundToInt()
        if (dragging) return Action.DRAG
        if (abs(x - downX) > slop || abs(y - downY) > slop) moved = true
        return Action.NONE
    }

    fun up(): Action = when {
        dragging -> {
            dragging = false
            Action.DROP
        }
        !moved -> Action.TAP
        else -> Action.NONE
    }

    fun cancel(): Action {
        if (!dragging) return Action.NONE
        dragging = false
        return Action.DROP
    }
}

/**
 * 손잡이 뷰의 터치 리스너. 탭은 view.performClick() 으로 넘겨 기존 클릭 동작(띠 펼치기)과 접근성 클릭이 같은 길을 탄다.
 * 좌표는 rawX·rawY(화면 기준)라 끄는 동안 창이 손가락을 따라 움직여도 흔들리지 않는다.
 */
class GuideOverlayHandleDrag(
    private val view: View,
    private val onDragStart: () -> Unit,
    private val onDrag: (dx: Int, dy: Int) -> Unit,
    private val onDrop: () -> Unit,
) : View.OnTouchListener {
    private val gesture = HandleGesture(ViewConfiguration.get(view.context).scaledTouchSlop.toFloat())
    private val handler = Handler(Looper.getMainLooper())
    private val longPress = Runnable {
        if (gesture.longPress() == HandleGesture.Action.DRAG_START) {
            view.performHapticFeedback(HapticFeedbackConstants.LONG_PRESS)
            onDragStart()
        }
    }

    override fun onTouch(v: View, event: MotionEvent): Boolean = when (event.actionMasked) {
        MotionEvent.ACTION_DOWN -> {
            gesture.down(event.rawX, event.rawY)
            handler.postDelayed(longPress, ViewConfiguration.getLongPressTimeout().toLong())
            true
        }
        MotionEvent.ACTION_MOVE -> {
            if (gesture.move(event.rawX, event.rawY) == HandleGesture.Action.DRAG) {
                onDrag(gesture.dx, gesture.dy)
            } else if (gesture.moved) {
                handler.removeCallbacks(longPress)
            }
            true
        }
        MotionEvent.ACTION_UP -> {
            handler.removeCallbacks(longPress)
            when (gesture.up()) {
                // 클릭이 손잡이 창을 지우므로(띠로 바뀜) 프레임워크 클릭처럼 터치 처리가 끝난 뒤로 미룬다
                HandleGesture.Action.TAP -> v.post { v.performClick() }
                HandleGesture.Action.DROP -> onDrop()
                else -> Unit
            }
            true
        }
        MotionEvent.ACTION_CANCEL -> {
            handler.removeCallbacks(longPress)
            if (gesture.cancel() == HandleGesture.Action.DROP) onDrop()
            true
        }
        else -> false // ACTION_OUTSIDE 등은 소비하지 않는다
    }

    /** 창을 내릴 때 대기 중인 길게 누르기를 지운다. */
    fun release() {
        handler.removeCallbacks(longPress)
    }
}
