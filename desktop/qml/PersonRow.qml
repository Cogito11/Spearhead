import QtQuick
import QtQuick.Controls

AbstractButton {
    id: control
    required property var item
    signal contextRequested(string identity, real px, real py)
    height: Theme.px(72)
    width: ListView.view ? ListView.view.width : implicitWidth
    focusPolicy: Qt.StrongFocus
    background: Rectangle {
        radius: Theme.px(16)
        color: control.item.selected ? Theme.soft : control.hovered ? Theme.soft : "transparent"
        border.color: control.item.selected ? Theme.line : "transparent"
        Rectangle {
            visible: control.item.selected
            width: Theme.px(3)
            height: parent.height - Theme.px(20)
            radius: width / 2
            color: Theme.primary
            anchors.left: parent.left
            anchors.verticalCenter: parent.verticalCenter
        }
    }
    contentItem: Item {
        Avatar {
            id: avatar
            title: control.item.title
            identity: control.item.key
            nearby: !!control.item.nearby
            anchors.left: parent.left
            anchors.leftMargin: Theme.px(12)
            anchors.verticalCenter: parent.verticalCenter
        }
        Column {
            anchors.left: avatar.right
            anchors.leftMargin: Theme.px(12)
            anchors.right: parent.right
            anchors.rightMargin: Theme.px(12)
            anchors.verticalCenter: parent.verticalCenter
            Row {
                width: parent.width
                SLabel {
                    text: control.item.title || ""
                    size: 16
                    font.weight: control.item.unread ? Font.Bold : Font.DemiBold
                    width: parent.width - timestamp.width - Theme.px(6)
                    elide: Text.ElideRight
                }
                SLabel {
                    id: timestamp
                    text: control.item.time || ""
                    size: 12
                    color: Theme.muted
                    anchors.verticalCenter: parent.verticalCenter
                }
            }
            Row {
                width: parent.width
                spacing: Theme.px(8)
                SLabel {
                    text: control.item.subtitle || ""
                    size: 14
                    color: control.item.unread ? Theme.fg : Theme.muted
                    width: parent.width - badge.width - parent.spacing
                    elide: Text.ElideRight
                }
                Rectangle {
                    id: badge
                    visible: !!control.item.unread
                    width: visible ? Math.max(Theme.px(23), count.implicitWidth + Theme.px(10)) : 0
                    height: Theme.px(22)
                    radius: height / 2
                    color: Theme.primary
                    anchors.verticalCenter: parent.verticalCenter
                    SLabel {
                        id: count
                        text: control.item.unread || ""
                        size: 12
                        color: Theme.primaryText
                        font.weight: Font.Bold
                        anchors.centerIn: parent
                    }
                }
            }
        }
    }
    MouseArea {
        anchors.fill: parent
        acceptedButtons: Qt.RightButton
        onPressed: function (mouse) {
            var point = mapToItem(null, mouse.x, mouse.y);
            control.contextRequested(control.item.key, point.x, point.y);
        }
    }
    Keys.onPressed: function (event) {
        if (event.key === Qt.Key_Menu || (event.key === Qt.Key_F10 && event.modifiers & Qt.ShiftModifier)) {
            var p = mapToItem(null, width / 2, height);
            contextRequested(item.key, p.x, p.y);
            event.accepted = true;
        }
    }
}
