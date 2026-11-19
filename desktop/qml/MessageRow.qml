import QtQuick
import QtQuick.Controls
import SpearHead 1.0

Item {
    id: row
    required property var item
    signal contextRequested(var message, real px, real py, string selection)
    signal saveFile(string identity, string name)
    property bool outgoing: item.dir === "out"
    property bool fileMessage: !!item.file
    property bool hiddenMessage: !!item.hidden
    width: ListView.view ? ListView.view.width : 600
    height: item.kind === "day" ? Theme.px(54) : bubble.height + Theme.px(12)
    Row {
        visible: row.item.kind === "day"
        width: parent.width
        y: Theme.px(14)
        spacing: Theme.px(16)
        Rectangle {
            color: Theme.line
            height: 1
            width: (parent.width - date.implicitWidth - 2 * parent.spacing) / 2
            anchors.verticalCenter: parent.verticalCenter
        }
        SLabel {
            id: date
            text: row.item.label || ""
            size: 12
            color: Theme.muted
        }
        Rectangle {
            color: Theme.line
            height: 1
            width: (parent.width - date.implicitWidth - 2 * parent.spacing) / 2
            anchors.verticalCenter: parent.verticalCenter
        }
    }
    Rectangle {
        id: bubble
        objectName: "message-" + row.item.key
        visible: row.item.kind !== "day"
        property real natural: row.fileMessage ? Math.max(176, backend.textWidth(row.item.file.name, Theme.px(15)) / Theme.scale + 34) : row.hiddenMessage ? 300 : backend.textWidth(row.item.body || "", Theme.px(16)) / Theme.scale + 32
        width: Math.min(row.width * 0.85, Theme.px(Math.max(120, natural)))
        height: body.implicitHeight + metadata.implicitHeight + Theme.px(28)
        x: row.outgoing ? row.width - width : 0
        color: "transparent"
        radius: Theme.px(18)
        border.color: "transparent"
        NativeBubble {
            anchors.fill: parent
            color: row.outgoing ? "#0b5cad" : Theme.bubble
            stroke: row.outgoing ? "#195fac" : Theme.line
            outgoing: row.outgoing
            rounding: Theme.px(18)
        }
        SoftShadow {
            anchors.fill: parent
            rounding: parent.radius
            z: -1
        }
        Column {
            id: body
            x: Theme.px(15)
            y: Theme.px(10)
            width: parent.width - Theme.px(30)
            spacing: Theme.px(6)
            TextEdit {
                id: bodyText
                objectName: "messageText-" + row.item.key
                visible: !row.fileMessage && !row.hiddenMessage
                width: parent.width
                text: row.item.rich || ""
                textFormat: TextEdit.RichText
                readOnly: true
                selectByMouse: true
                wrapMode: TextEdit.Wrap
                color: row.outgoing ? "#ffffff" : Theme.fg
                selectionColor: Theme.primary
                selectedTextColor: Theme.primaryText
                font.family: Theme.family
                font.pixelSize: Theme.px(16)
                renderType: Text.NativeRendering
                onLinkActivated: function (link) {
                    backend.openLink(link);
                }
                Accessible.name: row.item.body || ""
                Keys.onPressed: function (event) {
                    if (event.key === Qt.Key_Menu || (event.key === Qt.Key_F10 && event.modifiers & Qt.ShiftModifier)) {
                        var p = mapToItem(null, width / 2, height);
                        row.contextRequested(row.item, p.x, p.y, selectedText);
                        event.accepted = true;
                    }
                }
            }
            SButton {
                visible: row.hiddenMessage && !row.outgoing
                width: parent.width
                text: "Open read-once message"
                onClicked: backend.reveal(row.item.id)
                compact: true
            }
            SLabel {
                visible: row.hiddenMessage && row.outgoing
                width: parent.width
                wrapMode: Text.Wrap
                text: "Read-once message"
                size: 14
                color: "#deebf8"
            }
            Row {
                visible: row.fileMessage
                spacing: Theme.px(8)
                width: parent.width
                NativeGlyph {
                    name: "attach"
                    color: row.outgoing ? "white" : Theme.fg
                    width: Theme.px(16)
                    height: width
                    anchors.verticalCenter: parent.verticalCenter
                }
                SLabel {
                    text: row.fileMessage ? row.item.file.name : ""
                    size: 15
                    font.weight: Font.DemiBold
                    color: row.outgoing ? "white" : Theme.fg
                    width: parent.width - Theme.px(24)
                    wrapMode: Text.Wrap
                }
            }
            SLabel {
                visible: row.fileMessage
                text: row.fileMessage ? backend.formatSize(row.item.file.size) : ""
                size: 14
                color: row.outgoing ? "white" : Theme.fg
            }
            SButton {
                visible: row.fileMessage
                text: "Save file…"
                compact: true
                onClicked: row.saveFile(row.item.id, row.item.file.name)
            }
            SLabel {
                visible: !!row.item.gap
                text: "Some earlier messages are missing from this conversation."
                size: 12
                color: row.outgoing ? "#deebf8" : Theme.warn
                width: parent.width
                wrapMode: Text.Wrap
            }
        }
        SLabel {
            id: metadata
            x: Theme.px(15)
            anchors.top: body.bottom
            anchors.topMargin: Theme.px(3)
            width: parent.width - Theme.px(30)
            text: (row.item.burn ? (row.item.hidden ? "Read once · " : "Read once · now deleted · ") : "") + (row.item.time || "") + (row.outgoing && row.item.delivery ? "  " + row.item.delivery : "")
            size: 11
            color: row.outgoing ? "#deebf8" : Theme.muted
            horizontalAlignment: Text.AlignRight
            elide: Text.ElideRight
        }
        MouseArea {
            anchors.fill: parent
            acceptedButtons: Qt.RightButton
            onPressed: function (mouse) {
                var point = mapToItem(null, mouse.x, mouse.y);
                row.contextRequested(row.item, point.x, point.y, bodyText.selectedText);
            }
        }
    }
}
