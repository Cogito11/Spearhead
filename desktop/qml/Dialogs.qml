import QtQuick
import QtQuick.Controls
import QtQuick.Layouts

Item {
    id: dialogs

    property bool anyOpen: add.visible || settings.visible || help.visible || verify.visible || secret.visible || ask.visible || rename.visible
    property var findings: []
    property string renamePub: ""
    property string verifyPub: ""

    function showHelp() {
        help.open();
    }
    function showRename() {
        renamePub = backend.current;
        renameInput.text = backend.contact.label || "";
        rename.open();
        renameInput.selectAll();
        renameInput.forceActiveFocus();
    }
    function showSettings() {
        profileName.text = backend.state.me.name || "";
        retention.text = String(backend.state.settings.retention_days || 0);
        secretCheck.checked = !!backend.state.settings.secret_detection;
        settings.open();
    }

    Connections {
        function onCloseDialog() {
            add.close();
        }
        function onConfirmation(title, text, button, danger) {
            ask.title = title;
            askText.text = text;
            askYes.text = button;
            askYes.kind = danger ? "danger" : "primary";
            ask.open();
        }
        function onDialogChanged() {
            if (backend.found.pub)
                foundLabel.text = backend.found.name || "";
        }
        function onOpenDialog(name) {
            if (name === "add") {
                lookupInput.text = "";
                add.open();
            } else if (name === "verify") {
                dialogs.verifyPub = backend.current;
                verifyMismatch.visible = false;
                verify.open();
            } else if (name === "settings") {
                dialogs.showSettings();
            } else if (name === "help") {
                help.open();
            }
        }
        function onSecrets(items) {
            dialogs.findings = items;
            secret.approved = false;
            secret.open();
        }
        function onSettingsSaved() {
            settings.close();
        }

        target: backend
    }
    SDialog {
        id: settings
        objectName: "settingsDialog"
        title: "Settings"

        contentItem: SScrollView {
            id: settingsScroll
            clip: true
            contentWidth: availableWidth
            implicitHeight: settingsContent.implicitHeight
            objectName: "settingsScroll"

            Column {
                id: settingsContent
                spacing: Theme.px(12)
                width: settingsScroll.availableWidth - Theme.px(4)
                x: Theme.px(2)

                SLabel {
                    font.weight: Font.Bold
                    text: "Appearance"
                }
                SLabel {
                    color: Theme.muted
                    size: 13
                    text: "Choose the look that feels right. These preferences save automatically."
                    width: parent.width
                    wrapMode: Text.Wrap
                }
                RowLayout {
                    spacing: Theme.px(10)
                    width: parent.width

                    Repeater {
                        model: [{
                                "key": "system",
                                "title": "System",
                                "symbol": "system"
                            }, {
                                "key": "light",
                                "title": "Light",
                                "symbol": "sun"
                            }, {
                                "key": "dark",
                                "title": "Dark",
                                "symbol": "moon"
                            }]

                        delegate: AbstractButton {
                            required property var modelData

                            Layout.fillWidth: true
                            Layout.minimumWidth: 0
                            Layout.preferredWidth: 1
                            implicitHeight: Theme.px(80)
                            objectName: "theme-" + modelData.key

                            background: Rectangle {
                                border.color: backend.theme === modelData.key ? Theme.primary : Theme.line
                                color: backend.theme === modelData.key ? Theme.soft : Theme.bg
                                radius: Theme.px(14)
                            }
                            contentItem: Item {
                                Column {
                                    anchors.centerIn: parent
                                    spacing: Theme.px(6)

                                    NativeGlyph {
                                        anchors.horizontalCenter: parent.horizontalCenter
                                        color: backend.theme === modelData.key ? Theme.primary : Theme.muted
                                        height: width
                                        name: modelData.symbol
                                        width: Theme.px(24)
                                    }
                                    SLabel {
                                        anchors.horizontalCenter: parent.horizontalCenter
                                        color: backend.theme === modelData.key ? Theme.primary : Theme.muted
                                        font.weight: Font.DemiBold
                                        size: 14
                                        text: modelData.title
                                    }
                                }
                            }

                            onClicked: backend.theme = modelData.key
                        }
                    }
                }
                Row {
                    spacing: Theme.px(12)

                    SLabel {
                        font.weight: Font.DemiBold
                        height: Theme.px(44)
                        size: 16
                        text: "Text size"
                        width: Theme.px(96)
                    }
                    SButton {
                        text: "A−"

                        onClicked: backend.fontSize -= 10
                    }
                    SLabel {
                        height: Theme.px(44)
                        size: 16
                        text: backend.fontSize + "%"
                    }
                    SButton {
                        text: "A+"

                        onClicked: backend.fontSize += 10
                    }
                }
                Rectangle {
                    color: Theme.line
                    height: 1
                    width: parent.width
                }
                SLabel {
                    font.weight: Font.Bold
                    text: "Profile & messages"
                }
                SLabel {
                    size: 14
                    text: "Your name (what others see)"
                }
                SInput {
                    id: profileName
                    objectName: "settingsName"
                    width: parent.width
                }
                SLabel {
                    color: Theme.muted
                    size: 13
                    text: "A new name reaches others in a few seconds."
                    width: parent.width
                    wrapMode: Text.Wrap
                }
                SLabel {
                    color: Theme.muted
                    size: 13
                    text: "Your identity words: " + (backend.state.me.fingerprint || "")
                    width: parent.width
                    wrapMode: Text.Wrap
                }
                SLabel {
                    size: 14
                    text: "Keep messages for (days, 0 = forever)"
                }
                SInput {
                    id: retention
                    inputMethodHints: Qt.ImhDigitsOnly
                    objectName: "settingsRetention"
                    width: parent.width

                    validator: IntValidator {
                        bottom: 0
                        top: 3650
                    }
                }
                SCheck {
                    id: secretCheck
                    font.family: Theme.family
                    font.pixelSize: Theme.px(14)
                    text: "Warn before sending anything that looks like a password or key"
                    width: parent.width

                    contentItem: SLabel {
                        leftPadding: Theme.px(32)
                        size: 14
                        text: secretCheck.text
                        width: secretCheck.width - Theme.px(32)
                        wrapMode: Text.Wrap
                    }
                }
                SCheck {
                    id: closeToTray
                    checked: backend.closeToTray
                    objectName: "closeToTray"
                    text: "Close to system tray"
                    width: parent.width

                    contentItem: SLabel {
                        leftPadding: Theme.px(32)
                        size: 14
                        text: closeToTray.text
                        wrapMode: Text.Wrap
                    }

                    onToggled: backend.closeToTray = checked
                }
                SCheck {
                    id: alerts
                    checked: backend.notifications
                    text: "Desktop alerts for new messages and requests"
                    width: parent.width

                    contentItem: SLabel {
                        leftPadding: Theme.px(32)
                        size: 14
                        text: alerts.text
                        wrapMode: Text.Wrap
                    }

                    onToggled: backend.notifications = checked
                }
            }
        }
        footer: Item {
            implicitHeight: Theme.px(72)

            Row {
                anchors.right: parent.right
                anchors.rightMargin: Theme.px(24)
                anchors.verticalCenter: parent.verticalCenter
                spacing: Theme.px(10)

                SButton {
                    text: "Cancel"

                    onClicked: settings.close()
                }
                SButton {
                    enabled: !backend.saving
                    kind: "primary"
                    objectName: "settingsSave"
                    text: backend.saving ? "Saving…" : "Save"

                    onClicked: backend.saveSettings(profileName.text, parseInt(retention.text || "0"), secretCheck.checked)
                }
            }
        }
    }
    SDialog {
        id: add
        objectName: "addDialog"
        title: "Add a person"

        contentItem: SScrollView {
            id: addScroll
            clip: true
            contentWidth: availableWidth
            implicitHeight: addContent.implicitHeight

            Column {
                id: addContent
                spacing: Theme.px(12)
                width: addScroll.availableWidth

                SLabel {
                    font.weight: Font.Bold
                    text: "People on your network right now"
                    width: parent.width
                    wrapMode: Text.Wrap
                }
                Repeater {
                    id: nearby
                    model: (backend.state.nearby || []).filter(function (c) {
                            return !c.known;
                        })

                    delegate: Rectangle {
                        required property var modelData

                        border.color: Theme.line
                        color: Theme.bg
                        height: nearbyRow.implicitHeight + Theme.px(20)
                        radius: Theme.px(14)
                        width: addContent.width

                        RowLayout {
                            id: nearbyRow
                            spacing: Theme.px(10)
                            width: parent.width - Theme.px(20)
                            x: Theme.px(10)
                            y: Theme.px(10)

                            Avatar {
                                identity: modelData.pub
                                title: modelData.name || "?"
                            }
                            Column {
                                Layout.fillWidth: true

                                SLabel {
                                    elide: Text.ElideRight
                                    font.weight: Font.DemiBold
                                    text: modelData.name || "Unnamed"
                                    width: parent.width
                                }
                                SLabel {
                                    color: Theme.muted
                                    size: 12
                                    text: modelData.fingerprint
                                    width: parent.width
                                    wrapMode: Text.Wrap
                                }
                            }
                            SButton {
                                compact: true
                                text: "Add"

                                onClicked: backend.showFound(modelData)
                            }
                        }
                    }
                }
                SLabel {
                    color: Theme.muted
                    size: 14
                    text: "Nobody found yet. They need SpearHead open on the same network. You can also use an invite code below."
                    visible: nearby.count === 0
                    width: parent.width
                    wrapMode: Text.Wrap
                }
                Rectangle {
                    color: Theme.line
                    height: 1
                    width: parent.width
                }
                SLabel {
                    font.weight: Font.Bold
                    text: "Your invite code"
                }
                SLabel {
                    color: Theme.muted
                    size: 14
                    text: "Send this to someone so they can add you."
                }
                TextArea {
                    color: Theme.fg
                    font.family: Theme.family
                    font.pixelSize: Theme.px(13)
                    objectName: "inviteCode"
                    padding: Theme.px(12)
                    readOnly: true
                    selectByMouse: true
                    text: backend.state.me.invite || ""
                    width: parent.width
                    wrapMode: TextEdit.WrapAnywhere

                    background: Rectangle {
                        border.color: Theme.line
                        color: Theme.bg
                        radius: Theme.px(11)
                    }
                }
                SButton {
                    text: "Copy my code"

                    onClicked: backend.copy(backend.state.me.invite || "")
                }
                Rectangle {
                    color: Theme.line
                    height: 1
                    width: parent.width
                }
                SLabel {
                    font.weight: Font.Bold
                    text: "Got a code or address from someone?"
                    width: parent.width
                    wrapMode: Text.Wrap
                }
                SLabel {
                    size: 14
                    text: "Invite code or address (like 10.0.0.5:7777)"
                    width: parent.width
                    wrapMode: Text.Wrap
                }
                SInput {
                    id: lookupInput
                    objectName: "lookupInput"
                    width: parent.width

                    onAccepted: backend.lookup(text)
                }
                SButton {
                    enabled: !backend.lookingUp
                    objectName: "lookupButton"
                    text: backend.lookingUp ? "Looking up…" : "Look up"

                    onClicked: backend.lookup(lookupInput.text)
                }
                Column {
                    spacing: Theme.px(10)
                    visible: !!backend.found.pub
                    width: parent.width

                    SLabel {
                        font.weight: Font.Bold
                        text: "Found someone"
                    }
                    SLabel {
                        size: 14
                        text: (backend.found.name || "Someone") + " · identity words: " + (backend.found.fingerprint || "Not yet checked")
                        width: parent.width
                        wrapMode: Text.Wrap
                    }
                    SLabel {
                        color: Theme.warn
                        size: 14
                        text: "Only continue if you expected this person."
                        width: parent.width
                        wrapMode: Text.Wrap
                    }
                    SLabel {
                        size: 14
                        text: "What should you call them?"
                    }
                    SInput {
                        id: foundLabel
                        width: parent.width
                    }
                    SButton {
                        kind: "primary"
                        objectName: "addFound"
                        text: !backend.found.known ? "Add them" : backend.found.status === "pending" ? "Accept their request" : backend.found.status === "blocked" ? "Unblock" : "Open conversation"

                        onClicked: backend.addFound(foundLabel.text)
                    }
                }
            }
        }
    }
    SDialog {
        id: verify
        objectName: "verifyDialog"
        title: "Verify " + (backend.contact.label || "")

        contentItem: Column {
            spacing: Theme.px(16)

            SLabel {
                size: 15
                text: backend.contact.verified ? "You already verified this person. Here are the words again if you want to re-check them together." : "Call them, or meet in person, and read these words to each other. If you both see the same words in the same order, nobody has intercepted your connection."
                width: parent.width
                wrapMode: Text.Wrap
            }
            Rectangle {
                border.color: Theme.line
                color: Theme.bg
                height: code.implicitHeight + Theme.px(28)
                radius: Theme.px(11)
                width: parent.width

                SLabel {
                    id: code
                    anchors.centerIn: parent
                    font.family: "DejaVu Sans Mono"
                    font.letterSpacing: Theme.px(1)
                    font.weight: Font.DemiBold
                    objectName: "safetyWords"
                    text: backend.safetyCode
                    width: parent.width - Theme.px(28)
                    wrapMode: Text.Wrap
                }
            }
            SLabel {
                id: verifyMismatch
                color: Theme.danger
                size: 14
                text: "Don't trust this person yet. Different words can mean someone is in the middle. Check with them another way."
                visible: false
                width: parent.width
                wrapMode: Text.Wrap
            }
        }
        footer: Item {
            implicitHeight: Theme.px(72)

            Flow {
                anchors.right: parent.right
                anchors.rightMargin: Theme.px(24)
                anchors.verticalCenter: parent.verticalCenter
                spacing: Theme.px(8)

                SButton {
                    text: backend.contact.verified ? "Close" : "Cancel"

                    onClicked: verify.close()
                }
                SButton {
                    kind: "danger"
                    text: "They don't match"
                    visible: !backend.contact.verified

                    onClicked: verifyMismatch.visible = true
                }
                SButton {
                    kind: "primary"
                    text: "They match"
                    visible: !backend.contact.verified

                    onClicked: {
                        backend.contactAction(dialogs.verifyPub, "verify");
                        verify.close();
                    }
                }
            }
        }
    }
    SDialog {
        id: secret

        property bool approved: false

        objectName: "secretDialog"
        title: "This looks like a password or key"

        contentItem: Column {
            spacing: Theme.px(12)

            Repeater {
                model: dialogs.findings

                delegate: SLabel {
                    required property var modelData

                    size: 14
                    text: "• " + modelData.rule + " (" + modelData.sample + ")"
                    width: parent.width
                    wrapMode: Text.Wrap
                }
            }
            SLabel {
                color: Theme.muted
                size: 14
                text: "Consider turning on Read once so this message is deleted after they open it."
                width: parent.width
                wrapMode: Text.Wrap
            }
        }
        footer: Item {
            implicitHeight: Theme.px(72)

            Row {
                anchors.right: parent.right
                anchors.rightMargin: Theme.px(24)
                anchors.verticalCenter: parent.verticalCenter
                spacing: Theme.px(10)

                SButton {
                    objectName: "sendAnyway"
                    text: "Send anyway"

                    onClicked: {
                        secret.approved = true;
                        secret.close();
                        backend.confirmSend(true);
                    }
                }
                SButton {
                    kind: "primary"
                    text: "Don't send"

                    onClicked: secret.close()
                }
            }
        }

        onClosed: if (!approved)
            backend.confirmSend(false)
    }
    SDialog {
        id: ask
        objectName: "confirmationDialog"

        contentItem: SLabel {
            id: askText
            size: 15
            wrapMode: Text.Wrap
        }
        footer: Item {
            implicitHeight: Theme.px(72)

            Row {
                anchors.right: parent.right
                anchors.rightMargin: Theme.px(24)
                anchors.verticalCenter: parent.verticalCenter
                spacing: Theme.px(10)

                SButton {
                    kind: askYes.kind === "danger" ? "primary" : "normal"
                    text: "Cancel"

                    onClicked: {
                        backend.confirmAction(false);
                        ask.close();
                    }
                }
                SButton {
                    id: askYes
                    objectName: "confirmYes"

                    onClicked: {
                        backend.confirmAction(true);
                        ask.close();
                    }
                }
            }
        }

        onRejected: backend.confirmAction(false)
    }
    SDialog {
        id: rename
        objectName: "renameDialog"
        title: "Rename"

        contentItem: Column {
            spacing: Theme.px(12)

            SLabel {
                size: 14
                text: "What should you call them?"
            }
            SInput {
                id: renameInput
                objectName: "renameInput"
                width: parent.width
            }
        }
        footer: Item {
            implicitHeight: Theme.px(72)

            Row {
                anchors.right: parent.right
                anchors.rightMargin: Theme.px(24)
                anchors.verticalCenter: parent.verticalCenter
                spacing: Theme.px(10)

                SButton {
                    text: "Cancel"

                    onClicked: rename.close()
                }
                SButton {
                    kind: "primary"
                    text: "Save"

                    onClicked: {
                        backend.contactAction(dialogs.renamePub, "rename", renameInput.text);
                        rename.close();
                    }
                }
            }
        }
    }
    SDialog {
        id: help
        objectName: "helpDialog"
        title: "Help"

        contentItem: SScrollView {
            id: helpScroll
            clip: true
            contentWidth: availableWidth
            implicitHeight: helpContent.implicitHeight
            objectName: "helpScroll"

            Column {
                id: helpContent
                spacing: Theme.px(12)
                width: helpScroll.availableWidth

                Repeater {
                    model: [{
                            "title": "Keep SpearHead running",
                            "body": "By default, closing the window keeps messaging and discovery running in the system tray and releases the UI. Uncheck Close to system tray in Settings to quit completely when closing. Launching SpearHead again reopens the existing instance. Use Quit SpearHead in the tray to stop it."
                        }, {
                            "title": "Nobody shows up under Add a person",
                            "body": "Both people need SpearHead open on the same network or VPN. Check firewall settings and Wi-Fi client isolation. You can also share an invite code."
                        }, {
                            "title": "What does Verified mean?",
                            "body": "Compare the safety words with that person over a trusted channel. Unverified people still work, but verify anyone you will share private things with."
                        }, {
                            "title": "What is Read once?",
                            "body": "The message can be revealed once locally and is then deleted from local storage. It cannot prevent screenshots or someone remembering it."
                        }, {
                            "title": "Messages waiting to send",
                            "body": "SpearHead retries when the other person becomes reachable. After seven days it gives up."
                        }, {
                            "title": "Files and local data",
                            "body": "Files are encrypted in transit and checked for integrity, with a 256 MiB limit. Linux history and identity files are protected by permissions, not encrypted at rest."
                        }]

                    delegate: Column {
                        required property var modelData

                        spacing: Theme.px(5)
                        width: helpContent.width

                        SLabel {
                            font.weight: Font.Bold
                            text: modelData.title
                            width: parent.width
                            wrapMode: Text.Wrap
                        }
                        SLabel {
                            color: Theme.muted
                            size: 14
                            text: modelData.body
                            width: parent.width
                            wrapMode: Text.Wrap
                        }
                    }
                }
            }
        }
        footer: Item {
            implicitHeight: Theme.px(72)

            SLabel {
                anchors.left: parent.left
                anchors.leftMargin: Theme.px(24)
                anchors.verticalCenter: parent.verticalCenter
                color: Theme.muted
                size: 13
                text: "Version " + backend.version
            }
            SButton {
                anchors.right: parent.right
                anchors.rightMargin: Theme.px(24)
                anchors.verticalCenter: parent.verticalCenter
                kind: "primary"
                text: "Close"

                onClicked: help.close()
            }
        }
    }
}
