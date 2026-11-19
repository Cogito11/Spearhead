import QtQuick
import QtQuick.Controls

MenuItem {
    id: control
    property string symbol: ""
    property bool danger: false
    height: Theme.px(42)
    leftPadding: Theme.px(12)
    rightPadding: Theme.px(12)
    background: Rectangle {
        color: control.highlighted ? Theme.soft : "transparent"
        radius: Theme.px(10)
    }
    contentItem: Row {
        spacing: Theme.px(12)
        NativeGlyph {
            name: control.symbol
            color: control.danger ? Theme.danger : Theme.muted
            width: Theme.px(18)
            height: width
            anchors.verticalCenter: parent.verticalCenter
            visible: control.symbol.length > 0
        }
        SLabel {
            text: control.text
            size: 14
            color: control.danger ? Theme.danger : Theme.fg
            anchors.verticalCenter: parent.verticalCenter
        }
    }
}
