import QtQuick
import QtQuick.Controls

TextField {
    id: control
    font.family: Theme.family
    font.pixelSize: Theme.px(16)
    color: Theme.fg
    placeholderTextColor: Theme.muted
    selectionColor: Theme.primary
    selectedTextColor: Theme.primaryText
    implicitHeight: Theme.px(44)
    leftPadding: Theme.px(13)
    rightPadding: Theme.px(13)
    selectByMouse: true
    background: Rectangle {
        color: Theme.surface
        radius: Theme.px(11)
        border.color: control.activeFocus ? Theme.primary : Theme.line
    }
}
