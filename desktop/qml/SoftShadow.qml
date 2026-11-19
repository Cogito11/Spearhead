import QtQuick

Item {
    property real rounding: Theme.px(18)
    Repeater {
        model: 5
        Rectangle {
            required property int index
            x: -(5 - index)
            y: 2 - (5 - index)
            width: parent.width + 2 * (5 - index)
            height: parent.height + 2 * (5 - index)
            radius: rounding + 5 - index
            color: Theme.dark ? "#070c16" : "#142a4a"
            opacity: 0.012 + index * 0.003
        }
    }
}
