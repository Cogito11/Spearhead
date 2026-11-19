import QtQuick
import QtQuick.Controls

Button {
    id: control
    property string kind: "normal"
    property bool compact: false
    property string iconName: ""
    implicitHeight: Theme.px(compact ? 36 : 44)
    implicitWidth: Math.max(Theme.px(60), label.implicitWidth + Theme.px(34) + (iconName ? Theme.px(26) : 0))
    padding: Theme.px(12)
    focusPolicy: Qt.StrongFocus
    opacity: enabled ? 1 : 0.55
    background: Rectangle {
        radius: Theme.px(11)
        color: control.kind === "primary" ? Theme.primary : Theme.surface
        border.color: control.activeFocus && control.focusReason === Qt.TabFocusReason ? Theme.primary : control.kind === "primary" ? Theme.primary : control.kind === "danger" ? Theme.danger : control.hovered ? Theme.muted : Theme.line
        border.width: control.activeFocus && control.focusReason === Qt.TabFocusReason ? 2 : 1
    }
    contentItem: Row {
        spacing: Theme.px(8)
        NativeGlyph {
            width: Theme.px(18)
            height: width
            name: control.iconName
            visible: !!control.iconName
            color: label.color
            anchors.verticalCenter: parent.verticalCenter
        }
        SLabel {
            id: label
            objectName: "buttonLabel"
            text: control.text
            size: control.compact ? 14 : 16
            font.weight: control.kind === "primary" ? Font.DemiBold : Font.Medium
            color: control.kind === "primary" ? Theme.primaryText : control.kind === "danger" ? Theme.danger : Theme.fg
            anchors.verticalCenter: parent.verticalCenter
        }
    }
}
