package kr.seoulroute.seoul_route

import android.content.Context
import kotlin.math.roundToInt

/**
 * 진하기 손잡이 위치(px). 손잡이 창이 Gravity.TOP|END 라 right 는 화면 오른쪽 끝에서, top 은 화면 위에서 잰 거리이고
 * WindowManager.LayoutParams 의 x·y 와 같다.
 */
data class HandleOffset(val right: Int, val top: Int)

/** 손잡이·띠 위치 계산(안드로이드 의존 없음). */
object HandlePlacement {
    /** 지름 size 손잡이가 화면(width × height) 안이고 minTop(상태 표시줄 아래) 이상에 있도록 자른다. */
    fun clamp(o: HandleOffset, width: Int, height: Int, size: Int, minTop: Int): HandleOffset = HandleOffset(
        o.right.coerceIn(0, maxOf(0, width - size)),
        o.top.coerceIn(minTop, maxOf(minTop, height - size)),
    )

    /** 끌기 시작 위치 start 에서 손가락이 (dx, dy) 움직였을 때. right 는 오른쪽 끝 기준이라 dx 와 반대로 간다. */
    fun dragged(start: HandleOffset, dx: Int, dy: Int): HandleOffset = HandleOffset(start.right - dx, start.top + dy)

    /** 펼친 띠(높이 stripHeight)의 top. 손잡이 높이에 맞추되 화면 아래로 넘치지 않게 한다. */
    fun stripTop(handleTop: Int, stripHeight: Int, height: Int, minTop: Int): Int =
        handleTop.coerceIn(minTop, maxOf(minTop, height - stripHeight))
}

/** 사용자가 끌어 놓은 손잡이 위치를 dp 로 저장한다(화면 밀도가 바뀌어도 같은 자리). 앱 설정과 따로 둔다. */
class GuideOverlayHandleStore(context: Context) {
    private val prefs = context.getSharedPreferences(PREFS, Context.MODE_PRIVATE)
    private val density = context.resources.displayMetrics.density

    /** 저장된 위치(px). 끌어 본 적이 없으면 null. */
    fun load(): HandleOffset? {
        if (!prefs.contains(KEY_RIGHT_DP) || !prefs.contains(KEY_TOP_DP)) return null
        return HandleOffset(
            (prefs.getFloat(KEY_RIGHT_DP, 0f) * density).roundToInt(),
            (prefs.getFloat(KEY_TOP_DP, 0f) * density).roundToInt(),
        )
    }

    fun save(o: HandleOffset) {
        prefs.edit().putFloat(KEY_RIGHT_DP, o.right / density).putFloat(KEY_TOP_DP, o.top / density).apply()
    }

    companion object {
        private const val PREFS = "guide_overlay_handle"
        private const val KEY_RIGHT_DP = "right_dp"
        private const val KEY_TOP_DP = "top_dp"
    }
}
