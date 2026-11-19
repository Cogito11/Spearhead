import QtQuick
import QtQuick.Controls

AbstractButton {
    id: control
    property string symbol: ""
    property string tip: ""
    property color tint: Theme.muted
    property bool selected: false
    implicitWidth: Theme.px(44)
    implicitHeight: implicitWidth
    focusPolicy: Qt.StrongFocus
    opacity: enabled ? 1 : 0.45
    background: Rectangle {
        radius: width / 2
        color: control.selected ? Theme.primary : control.hovered ? Theme.soft : "transparent"
        border.width: control.activeFocus && control.focusReason === Qt.TabFocusReason ? 2 : 0
        border.color: Theme.muted
    }
    contentItem: Item {
        NativeGlyph {
            anchors.centerIn: parent
            name: control.symbol
            color: control.selected ? Theme.primaryText : control.tint
            width: Theme.px(22)
            height: width
        }
    }
    ToolTip.visible: hovered && tip.length > 0
    ToolTip.delay: 600
    ToolTip.text: tip
    Accessible.name: tip
}
