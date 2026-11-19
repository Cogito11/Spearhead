import QtQuick

Item {
    id: effect

    readonly property Binding behavior: Binding {
        property: "boundsBehavior"
        target: effect.flickable
        value: Flickable.DragAndOvershootBounds
    }
    readonly property Translate displacement: Translate {
        y: effect.offset

        Behavior on y  {
            enabled: effect.flickable && !effect.flickable.dragging && !effect.wheelReturn.running

            NumberAnimation {
                duration: 160
                easing.type: Easing.OutCubic
            }
        }
    }
    required property var flickable
    readonly property real limit: Theme.px(16)

    // Qt still reports overshoot with StopAtBounds. Paint only a small resisted
    // displacement, without moving the scroll position or scrollbar past its limits.
    // https://doc.qt.io/qt-6/qml-qtquick-flickable.html#boundsMovement-prop
    readonly property Binding movement: Binding {
        property: "boundsMovement"
        target: effect.flickable
        value: Flickable.StopAtBounds
    }
    readonly property real offset: Math.max(-limit, Math.min(limit, -limit * overshoot / (Math.abs(overshoot) + Theme.px(56)) + wheelOffset))
    readonly property real overshoot: flickable ? flickable.verticalOvershoot : 0
    readonly property Binding paint: Binding {
        property: "transform"
        target: effect.flickable ? effect.flickable.contentItem : null
        value: [effect.displacement]
    }
    // Observe wheel events without consuming them or changing native scrolling.
    // Qt does not report verticalOvershoot for these events at a fixed boundary.
    readonly property WheelHandler wheelFeedback: WheelHandler {
        acceptedDevices: PointerDevice.Mouse | PointerDevice.TouchPad
        blocking: false
        parent: effect.wheelSurface
        target: null

        onWheel: function (event) {
            event.accepted = false;
            var delta = event.pixelDelta.y || event.angleDelta.y;
            if (!delta || !effect.flickable)
                return;
            if ((delta > 0 && effect.flickable.atYBeginning) || (delta < 0 && effect.flickable.atYEnd)) {
                effect.wheelReturn.stop();
                var sign = delta > 0 ? 1 : -1;
                var previous = effect.wheelOffset * sign > 0 ? Math.abs(effect.wheelOffset) : 0;
                var pulse = effect.limit * Math.abs(delta) / (Math.abs(delta) + 40);
                effect.wheelOffset = sign * Math.min(effect.limit, previous + pulse);
                effect.wheelReturn.restart();
            }
        }
    }
    property real wheelOffset: 0
    readonly property NumberAnimation wheelReturn: NumberAnimation {
        duration: 210
        easing.type: Easing.OutCubic
        property: "wheelOffset"
        target: effect
        to: 0
    }
    property var wheelSurface: flickable
}
