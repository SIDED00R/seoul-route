package kr.seoulroute.seoul_route

import org.junit.Assert.assertEquals
import org.junit.Assert.assertFalse
import org.junit.Assert.assertTrue
import org.junit.Test

class HandleGestureTest {
    private val g = HandleGesture(slop = 10f)

    @Test
    fun shortPressIsTap() {
        g.down(100f, 100f)
        assertEquals(HandleGesture.Action.NONE, g.move(103f, 104f)) // slop 안 떨림
        assertEquals(HandleGesture.Action.TAP, g.up())
    }

    @Test
    fun longPressThenMoveDrags() {
        g.down(100f, 100f)
        assertEquals(HandleGesture.Action.DRAG_START, g.longPress())
        assertEquals(HandleGesture.Action.DRAG, g.move(60f, 250f))
        assertEquals(-40, g.dx)
        assertEquals(150, g.dy)
        assertEquals(HandleGesture.Action.DROP, g.up())
        assertFalse(g.dragging)
    }

    @Test
    fun movingBeforeLongPressIsNothing() {
        g.down(100f, 100f)
        assertEquals(HandleGesture.Action.NONE, g.move(100f, 130f))
        assertTrue(g.moved)
        assertEquals(HandleGesture.Action.NONE, g.longPress()) // 늦게 온 길게 누르기도 무시
        assertEquals(HandleGesture.Action.NONE, g.up())
    }

    @Test
    fun cancelDuringDragDrops() {
        g.down(0f, 0f)
        g.longPress()
        assertEquals(HandleGesture.Action.DROP, g.cancel())
        assertEquals(HandleGesture.Action.NONE, g.cancel())
    }

    @Test
    fun newPressResetsState() {
        g.down(0f, 0f)
        g.move(0f, 50f)
        g.up()
        g.down(0f, 0f)
        assertEquals(HandleGesture.Action.TAP, g.up())
    }
}

class HandlePlacementTest {
    @Test
    fun clampKeepsHandleOnScreenBelowStatusBar() {
        val size = 96
        val minTop = 80
        assertEquals(HandleOffset(0, minTop), HandlePlacement.clamp(HandleOffset(-50, 10), 1440, 3088, size, minTop))
        assertEquals(HandleOffset(1440 - size, 3088 - size),
            HandlePlacement.clamp(HandleOffset(5000, 9000), 1440, 3088, size, minTop))
        assertEquals(HandleOffset(300, 900), HandlePlacement.clamp(HandleOffset(300, 900), 1440, 3088, size, minTop))
    }

    @Test
    fun dragMovesRightOffsetOpposite() {
        // 손가락이 왼쪽(-dx)으로 가면 오른쪽 끝에서의 거리는 늘어난다
        assertEquals(HandleOffset(140, 350), HandlePlacement.dragged(HandleOffset(100, 200), -40, 150))
    }

    @Test
    fun stripFollowsHandleButStaysOnScreen() {
        assertEquals(900, HandlePlacement.stripTop(900, 168, 3088, 80))
        assertEquals(3088 - 168, HandlePlacement.stripTop(3000, 168, 3088, 80))
        assertEquals(80, HandlePlacement.stripTop(10, 168, 3088, 80))
    }
}
