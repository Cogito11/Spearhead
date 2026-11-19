import QtQuick

Rectangle {
    property string title: ""
    property string identity: ""
    readonly property string cleanTitle: title.replace(/\s+\(You\)$/, "")
    property bool nearby: false
    width: Theme.px(44)
    height: width
    radius: width / 2
    color: backend.avatarColor(cleanTitle, identity)
    SLabel {
        text: backend.initials(cleanTitle)
        color: "white"
        font.weight: Font.DemiBold
        size: 18
        anchors.centerIn: parent
    }
    Rectangle {
        visible: parent.nearby
        width: Theme.px(10)
        height: width
        radius: width / 2
        color: Theme.primary
        border.color: Theme.surface
        border.width: 2
        anchors.right: parent.right
        anchors.bottom: parent.bottom
    }
}
