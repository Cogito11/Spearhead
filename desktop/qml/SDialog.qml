import QtQuick
import QtQuick.Controls

Dialog {
    id: control
    parent: Overlay.overlay
    modal: true
    focus: true
    width: Math.min(Theme.px(576), parent.width - Theme.px(32))
    height: Math.min(implicitHeight, parent.height - Theme.px(32))
    x: (parent.width - width) / 2
    y: (parent.height - height) / 2
    padding: Theme.px(24)
    topPadding: Theme.px(12)
    closePolicy: Popup.CloseOnEscape | Popup.CloseOnPressOutside
    background: Rectangle {
        color: Theme.surface
        radius: Theme.px(18)
        border.color: Theme.line
        SoftShadow {
            anchors.fill: parent
            rounding: parent.radius
            z: -1
        }
    }
    Overlay.modal: Rectangle {
        color: "#99080a0e"
    }
    header: Item {
        implicitHeight: Theme.px(68)
        SLabel {
            text: control.title
            size: 20
            font.weight: Font.Bold
            anchors.left: parent.left
            anchors.leftMargin: Theme.px(24)
            anchors.verticalCenter: parent.verticalCenter
            width: parent.width - Theme.px(84)
            elide: Text.ElideRight
        }
        IconButton {
            symbol: "close"
            tip: "Close"
            anchors.right: parent.right
            anchors.rightMargin: Theme.px(12)
            anchors.verticalCenter: parent.verticalCenter
            onClicked: control.close()
        }
    }
}
