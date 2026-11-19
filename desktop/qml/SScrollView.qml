import QtQuick
import QtQuick.Controls

ScrollView {
    id: control

    property ScrollEdge edgeEffect: ScrollEdge {
        flickable: control.contentItem
        wheelSurface: control
    }

    clip: true
}
