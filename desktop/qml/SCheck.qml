import QtQuick
import QtQuick.Controls

CheckBox {
    id: control
    implicitHeight: Math.max(Theme.px(32), contentItem.contentHeight + Theme.px(8))
    indicator: Rectangle {
        width: Theme.px(20)
        height: width
        x: 0
        y: (control.height - height) / 2
        radius: Theme.px(4)
        color: control.checked ? Theme.primary : Theme.surface
        border.color: control.checked ? Theme.primary : Theme.line
        NativeGlyph {
            name: "check"
            visible: control.checked
            color: Theme.primaryText
            width: Theme.px(16)
            height: width
            anchors.centerIn: parent
        }
    }
    contentItem: SLabel {
        text: control.text
        size: 14
        leftPadding: Theme.px(32)
        wrapMode: Text.Wrap
    }
}
