import QtQuick
import QtQuick.Controls
import QtQuick.Layouts

Rectangle {
    id: control
    required property var item
    width: parent ? parent.width : Theme.px(800)
    height: content.implicitHeight + Theme.px(44)
    radius: Theme.px(19)
    color: Theme.surface
    border.color: Theme.line
    SoftShadow {
        anchors.fill: parent
        rounding: parent.radius
        z: -1
    }
    Column {
        id: content
        anchors.left: parent.left
        anchors.right: parent.right
        anchors.margins: Theme.px(22)
        y: Theme.px(20)
        spacing: Theme.px(12)
        SLabel {
            text: control.item.title
            font.weight: Font.Bold
            width: parent.width
            wrapMode: Text.Wrap
        }
        SLabel {
            visible: !!control.item.description
            text: control.item.description || ""
            color: Theme.muted
            width: parent.width
            wrapMode: Text.Wrap
        }
        Repeater {
            model: control.item.fields || []
            delegate: Row {
                required property var modelData
                width: content.width
                spacing: Theme.px(18)
                SLabel {
                    text: modelData.label
                    size: 15
                    color: Theme.muted
                    font.weight: Font.Medium
                    width: Math.max(Theme.px(112), parent.width / 4 - Theme.px(4))
                    wrapMode: Text.Wrap
                    verticalAlignment: Text.AlignTop
                }
                TextEdit {
                    text: modelData.value
                    width: parent.width - parent.children[0].width - parent.spacing
                    readOnly: true
                    selectByMouse: true
                    wrapMode: TextEdit.Wrap
                    textFormat: TextEdit.PlainText
                    color: Theme.fg
                    font.family: Theme.family
                    font.pixelSize: Theme.px(15)
                    selectionColor: Theme.primary
                    selectedTextColor: Theme.primaryText
                    renderType: Text.NativeRendering
                    Accessible.name: modelData.label
                }
            }
        }
        SButton {
            visible: !!control.item.scan
            objectName: "networkScan"
            text: backend.network.scanning ? "Scanning…" : backend.network.identifying ? "Identifying…" : "Scan local devices"
            kind: "primary"
            enabled: !backend.network.scanning && !backend.network.identifying
            onClicked: backend.scan()
        }
        SButton {
            visible: !!control.item.missing
            text: "View LAN information"
            onClicked: backend.showDetail("lan")
        }
        SButton {
            visible: !!control.item.contactPub
            text: control.item.known ? "View contact" : "Add contact"
            onClicked: {
                if (control.item.known)
                    backend.viewContact(control.item.contactPub);
                else {
                    backend.beginAdd();
                    backend.showFound({
                            "pub": control.item.contactPub,
                            "name": control.item.device.broadcast_name,
                            "addr": control.item.device.address,
                            "fingerprint": ""
                        });
                }
            }
        }
    }
}
