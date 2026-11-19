#include "bridge.h"
#include "style.h"
#include <QApplication>
#include <QtDBus>
#include <algorithm>

QVariant MessageModel::data(const QModelIndex &index, int role) const {
  return index.isValid() && index.row() < rows.size() && role == Qt::UserRole + 1
             ? QVariant(rows[index.row()])
             : QVariant();
}
void MessageModel::update(const QList<QVariantMap> &next) {
  for (int i = 0; i < next.size(); i++) {
    if (i >= rows.size() || rows[i].value("key") != next[i].value("key")) {
      int found = -1;
      for (int j = i + 1; j < rows.size(); j++)
        if (rows[j].value("key") == next[i].value("key")) {
          found = j;
          break;
        }
      if (found >= 0) {
        beginMoveRows({}, found, found, {}, i);
        rows.move(found, i);
        endMoveRows();
      } else {
        beginInsertRows({}, i, i);
        rows.insert(i, next[i]);
        endInsertRows();
      }
    }
    if (rows[i] != next[i]) {
      rows[i] = next[i];
      emit dataChanged(index(i), index(i), {Qt::UserRole + 1});
    }
  }
  if (rows.size() > next.size()) {
    beginRemoveRows({}, next.size(), rows.size() - 1);
    rows.erase(rows.begin() + next.size(), rows.end());
    endRemoveRows();
  }
}
static QString text(const QJsonObject &o, const char *key) {
  return o.value(key).toString();
}
static QString joined(const QJsonValue &v) {
  QStringList values;
  for (const auto &x : v.toArray())
    values << x.toString();
  return values.join(", ");
}
static QString fullTime(const QJsonValue &value) {
  if (value.toDouble() <= 0)
    return "Not seen yet";
  QLocale locale;
  QString date = locale.dateFormat(QLocale::ShortFormat);
  date.replace(QRegularExpression("(?<!y)yy(?!y)"), "yyyy");
  QString time = locale.timeFormat(QLocale::ShortFormat);
  if (!time.contains('s'))
    time.replace("mm", "mm:ss");
  return locale.toString(QDateTime::fromSecsSinceEpoch(qint64(value.toDouble())),
                         date + ", " + time);
}

Bridge::Bridge(const QJsonObject &payload, QObject *parent)
    : QObject(parent), api(QUrl(text(payload, "url"))), m_messages(this), m_people(this) {
  initialiseFonts();
  connect(this, &Bridge::peopleChanged, this, [this] {
    QList<QVariantMap> rows;
    for (auto value : people())
      rows << value.toMap();
    m_people.update(rows);
  });
  m_restore = payload.value("session").toObject();
  m_theme = m_restore.value("theme").toString("system");
  if (!QStringList{"light", "dark", "system"}.contains(m_theme))
    m_theme = "system";
  m_fontSize = qBound(80, m_restore.value("fontSize").toInt(100), 160);
  m_notifications = m_restore.value("notifications").toBool(true);
  m_closeToTray = m_restore.value("closeToTray").toBool(true);
  m_systemDark = payload.value("dark").toBool(
      QApplication::palette().color(QPalette::Window).lightness() < 128);
  m_state = {{"me", QJsonObject{{"name", ""}, {"pub", ""}, {"invite", ""}, {"fingerprint", ""}}},
             {"contacts", QJsonArray{}},
             {"requests", QJsonArray{}},
             {"nearby", QJsonArray{}},
             {"settings", QJsonObject{{"retention_days", 90}, {"secret_detection", true}}},
             {"listening", true},
             {"port", 7777}};
  m_network = {{"devices", QJsonArray{}}, {"interfaces", QJsonArray{}}, {"dns", QJsonArray{}}};
  applyNativePalette(m_theme == "dark" || (m_theme == "system" && m_systemDark));
  m_timer.setInterval(2000);
  connect(&m_timer, &QTimer::timeout, this, &Bridge::refresh);
  auto portal =
      new QDBusInterface("org.freedesktop.portal.Desktop", "/org/freedesktop/portal/desktop",
                         "org.freedesktop.portal.Settings", QDBusConnection::sessionBus(), this);
  if (portal->isValid()) {
    auto watcher = new QDBusPendingCallWatcher(
        portal->asyncCall("Read", "org.freedesktop.appearance", "color-scheme"), this);
    connect(watcher, &QDBusPendingCallWatcher::finished, this, [this, watcher] {
      QDBusPendingReply<QDBusVariant> result = *watcher;
      if (!result.isError()) {
        m_systemDark = result.value().variant().toUInt() == 1;
        applyNativePalette(m_theme == "dark" || (m_theme == "system" && m_systemDark));
        emit appearanceChanged();
      }
      watcher->deleteLater();
    });
    QDBusConnection::sessionBus().connect("org.freedesktop.portal.Desktop",
                                          "/org/freedesktop/portal/desktop",
                                          "org.freedesktop.portal.Settings", "SettingChanged", this,
                                          SLOT(portalChanged(QString, QString, QDBusVariant)));
  }
}
void Bridge::start() {
  if (api.token.isEmpty())
    return;
  refresh();
  m_timer.start();
}
QJsonObject Bridge::findContact(const QString &pub) const {
  for (const auto &key : {"contacts", "requests"})
    for (const auto &item : m_state.value(key).toArray())
      if (text(item.toObject(), "pub") == pub)
        return item.toObject();
  return {};
}
QVariantMap Bridge::contact() const {
  return findContact(m_current).toVariantMap();
}
QVariantList Bridge::people() const {
  QVariantList result;
  const QString query = m_sidebarSearch.trimmed().toCaseFolded();
  auto add = [&](QString key, QString title, QString subtitle, const QJsonObject &data) {
    bool match = query.isEmpty();
    if (!match) {
      for (const char *field : {"name",         "label",  "their_name",     "device",    "platform",
                                "fingerprint",  "pub",    "address",        "ip",        "mac",
                                "notes",        "status", "broadcast_name", "hostname",  "vendor",
                                "manufacturer", "model",  "kind",           "addresses", "macs",
                                "services"}) {
        auto value = data.value(field);
        QString s = value.isArray() ? joined(value) : value.toString();
        if (s.toCaseFolded().contains(query)) {
          match = true;
          break;
        }
      }
    }
    if (!match && key != "self" && key != "lan")
      return;
    QVariantMap row = data.toVariantMap();
    row["key"] = key;
    row["title"] = title;
    row["subtitle"] = subtitle;
    row["selected"] = key == (m_mode == "conversations" ? m_current : m_detail);
    row["time"] =
        data.value("last_ts").toDouble() > 0
            ? (QDateTime::fromSecsSinceEpoch(qint64(data.value("last_ts").toDouble())).date() ==
                       QDate::currentDate()
                   ? timeText(data.value("last_ts").toDouble())
                   : dateText(data.value("last_ts").toDouble()))
            : QString();
    result << row;
  };
  if (m_mode == "conversations") {
    QList<QJsonObject> contacts;
    for (auto value : m_state.value("contacts").toArray())
      contacts << value.toObject();
    std::sort(contacts.begin(), contacts.end(), [](const auto &a, const auto &b) {
      if (a.value("unread") != b.value("unread"))
        return a.value("unread").toInt() > b.value("unread").toInt();
      if (a.value("last_ts") != b.value("last_ts"))
        return a.value("last_ts").toDouble() > b.value("last_ts").toDouble();
      return QString::localeAwareCompare(text(a, "label"), text(b, "label")) < 0;
    });
    for (const auto &c : contacts)
      add(text(c, "pub"), text(c, "label"), text(c, "preview"), c);
  } else if (m_mode == "contacts") {
    auto me = m_state.value("me").toObject();
    add("self", text(me, "name") + " (You)", text(me, "device"), me);
    for (const char *key : {"contacts", "requests"})
      for (auto value : m_state.value(key).toArray()) {
        auto c = value.toObject();
        add(text(c, "pub"), text(c, "label"), text(c, "their_name") + " · " + text(c, "status"), c);
      }
  } else {
    int connections = 0;
    for (auto value : m_network.value("interfaces").toArray())
      if (value.toObject().value("up").toBool() && !value.toObject().value("virtual").toBool())
        connections++;
    add("lan", "LAN information",
        QString::number(m_network.value("devices").toArray().size()) + " devices · " +
            QString::number(connections) + " LAN connections",
        {});
    for (auto value : m_network.value("devices").toArray()) {
      auto d = value.toObject();
      QString name = text(d, "name");
      if (name.isEmpty())
        name = text(d, "broadcast_name");
      if (name.isEmpty())
        name = "Network device";
      if (d.value("self").toBool())
        name += " (You)";
      QString type = text(d, "kind");
      if (type.isEmpty())
        type = text(d, "status");
      add(text(d, "id"), name, text(d, "ip") + " · " + type, d);
    }
  }
  return result;
}
void Bridge::refresh() {
  if (m_refreshing || api.token.isEmpty())
    return;
  m_refreshing = true;
  api.get("/api/state", [this](int code, QJsonObject data) {
    m_refreshing = false;
    bool wasConnected = m_connected;
    m_connected = code == 200;
    if (code != 200) {
      if (wasConnected)
        emit stateChanged();
      return;
    }
    if (data != m_state || !wasConnected) {
      for (auto value : data.value("contacts").toArray()) {
        auto c = value.toObject();
        QString pub = text(c, "pub");
        int unread = c.value("unread").toInt();
        if (m_restored && unread > m_unread.value(pub) && !m_windowActive && m_notifications)
          event({{"event", "notification"}, {"text", "New message from " + text(c, "label")}});
        m_unread[pub] = unread;
      }
      m_state = data;
      if (m_mode == "contacts" && !m_editing && !m_saving)
        initializeEditor();
      emit stateChanged();
      emit peopleChanged();
      emit contactChanged();
      emit detailChanged();
    }
    if (!m_restored) {
      m_restored = true;
      restore();
    }
    if (!m_current.isEmpty() && findContact(m_current).isEmpty())
      back();
    loadThread();
    if (m_mode == "network")
      loadNetwork();
  });
}
void Bridge::restore() {
  for (auto value : m_restore.value("drafts").toArray()) {
    auto pair = value.toArray();
    if (pair.size() == 2 && pair[0].isString() && pair[1].isString())
      m_drafts[pair[0].toString()] = pair[1].toString();
  }
  QString mode = m_restore.value("panelMode").toString("conversations");
  setMode(mode);
  if (mode == "conversations" && !findContact(text(m_restore, "current")).isEmpty())
    select(text(m_restore, "current"));
  else if (mode != "conversations") {
    showDetail(m_restore.value("detailSelection").toString(mode == "contacts" ? "self" : "lan"));
    auto edit = m_restore.value("editing").toObject();
    if (!edit.isEmpty()) {
      m_editLabel = text(edit, "label");
      m_editAddress = text(edit, "address");
      m_editNotes = text(edit, "notes");
      m_editing = true;
      emit editorChanged();
    }
  }
  m_burn = m_restore.value("burn").toBool();
  setSidebarSearch(text(m_restore, "sidebarSearch"));
  emit draftChanged();
  m_restore = {};
}
void Bridge::setMode(const QString &value) {
  if (!QStringList{"conversations", "contacts", "network"}.contains(value))
    return;
  if (!m_current.isEmpty())
    m_drafts[m_current] = m_draft;
  if (value != m_mode) {
    m_generation++;
    m_revealed.clear();
    m_thread = {};
    m_messages.update({});
    m_current.clear();
    m_draft.clear();
    m_burn = false;
    setSearchOpen(false);
  }
  m_mode = value;
  m_sidebarSearch.clear();
  m_detail = value == "contacts" ? "self" : "lan";
  initializeEditor();
  emit navigationChanged();
  emit contactChanged();
  emit peopleChanged();
  emit detailChanged();
  emit draftChanged();
  if (value == "network")
    loadNetwork();
}
void Bridge::setSidebarSearch(const QString &value) {
  if (m_sidebarSearch != value) {
    m_sidebarSearch = value;
    emit peopleChanged();
  }
}
void Bridge::select(const QString &pub) {
  if (findContact(pub).isEmpty())
    return;
  if (!m_current.isEmpty())
    m_drafts[m_current] = m_draft;
  if (m_mode != "conversations")
    m_sidebarSearch.clear();
  m_mode = "conversations";
  if (pub != m_current) {
    m_generation++;
    m_revealed.clear();
    m_thread = {};
    m_search.clear();
    m_searchOpen = false;
    m_burn = false;
    m_draft = m_drafts.value(pub);
  }
  m_current = pub;
  emit navigationChanged();
  emit contactChanged();
  emit peopleChanged();
  emit draftChanged();
  emit searchChanged();
  updateMessages(true);
  loadThread();
}
void Bridge::back() {
  if (!m_current.isEmpty())
    m_drafts[m_current] = m_draft;
  m_current.clear();
  m_generation++;
  m_revealed.clear();
  m_thread = {};
  m_draft.clear();
  setSearchOpen(false);
  updateMessages(true);
  emit navigationChanged();
  emit contactChanged();
  emit peopleChanged();
  emit draftChanged();
}
void Bridge::setDraft(const QString &value) {
  if (value != m_draft) {
    m_draft = value;
    if (!m_current.isEmpty())
      m_drafts[m_current] = value;
    emit draftChanged();
  }
}
void Bridge::setBurn(bool value) {
  if (m_burn != value) {
    m_burn = value;
    emit draftChanged();
  }
}
void Bridge::setSearch(const QString &value) {
  if (m_search != value) {
    m_search = value;
    emit searchChanged();
    updateMessages(true);
  }
}
void Bridge::setSearchOpen(bool value) {
  if (m_searchOpen == value && (value || m_search.isEmpty()))
    return;
  m_searchOpen = value;
  bool hadQuery = !m_search.isEmpty();
  if (!value)
    m_search.clear();
  emit searchChanged();
  if (!value && hadQuery)
    updateMessages(true);
}
void Bridge::loadThread() {
  if (m_current.isEmpty())
    return;
  QString pub = m_current;
  int version = m_generation, request = ++m_threadRequest;
  api.get("/api/thread?c=" + QString::fromUtf8(QUrl::toPercentEncoding(pub)),
          [this, pub, version, request](int code, QJsonObject data) {
            if (code != 200 || pub != m_current || version != m_generation ||
                request != m_threadRequest)
              return;
            QJsonArray next = data.value("messages").toArray();
            if (next != m_thread) {
              m_thread = next;
              updateMessages();
            }
          });
}
void Bridge::updateMessages(bool reset) {
  QList<QJsonObject> messages;
  QSet<QString> ids;
  for (auto value : m_thread) {
    auto m = value.toObject();
    QString id = text(m, "id");
    ids.insert(id);
    messages << m_revealed.value(id, m);
  }
  for (auto it = m_revealed.begin(); it != m_revealed.end(); it++)
    if (!ids.contains(it.key()))
      messages << it.value();
  std::stable_sort(messages.begin(), messages.end(), [](const auto &a, const auto &b) {
    return a.value("ts").toDouble() < b.value("ts").toDouble();
  });
  QList<QVariantMap> rows;
  QString previousDay, query = m_search.trimmed().toCaseFolded();
  for (auto m : messages) {
    if (!query.isEmpty() &&
        (m.value("hidden").toBool() || !text(m, "body").toCaseFolded().contains(query)))
      continue;
    QString day = QDateTime::fromSecsSinceEpoch(qint64(m.value("ts").toDouble()))
                      .date()
                      .toString(Qt::ISODate);
    if (day != previousDay) {
      rows << QVariantMap{
          {"key", "day-" + day}, {"kind", "day"}, {"label", dateText(m.value("ts").toDouble())}};
      previousDay = day;
    }
    auto row = m.toVariantMap();
    row["key"] = text(m, "id");
    row["kind"] = "message";
    row["time"] = timeText(m.value("ts").toDouble());
    QString status = text(m, "state");
    row["delivery"] = status == "delivered" ? "✓ Delivered"
                      : status == "queued"  ? "Waiting to send"
                      : status == "failed"  ? "Couldn’t deliver"
                                            : "";
    row["rich"] = richText(text(m, "body"));
    rows << row;
  }
  if (rows != m_messages.rows || reset) {
    emit messagesChanging(reset);
    m_messages.update(rows);
    emit messagesChanged(reset);
  }
}
void Bridge::send() {
  if (m_sending || m_draft.trimmed().isEmpty() ||
      text(findContact(m_current), "status") != "accepted")
    return;
  m_pendingSend = {{"pub", m_current},
                   {"draft", m_draft},
                   {"body", m_draft.trimmed()},
                   {"burn", m_burn},
                   {"generation", m_generation}};
  m_sending = true;
  emit busyChanged();
  deliver(m_pendingSend, false);
}
void Bridge::deliver(const QJsonObject &captured, bool force) {
  api.post("/api/send",
           {{"c", captured.value("pub")},
            {"body", captured.value("body")},
            {"burn", captured.value("burn")},
            {"force", force}},
           [this, captured](int code, QJsonObject data) {
             if (code == 409) {
               m_pendingSend = captured;
               emit secrets(data.value("findings").toArray().toVariantList());
               return;
             }
             m_sending = false;
             m_pendingSend = {};
             emit busyChanged();
             if (code >= 200 && code < 300) {
               if (m_current == text(captured, "pub") &&
                   m_generation == captured.value("generation").toInt() &&
                   m_draft == text(captured, "draft")) {
                 setDraft("");
                 setBurn(false);
               }
               QString status = text(data, "state");
               emit toast(status == "queued"
                              ? "Waiting to send. It will retry when they are reachable."
                          : status == "failed" ? "Couldn’t deliver: " + text(data, "reason")
                                               : "Message delivered",
                          status == "failed");
               refresh();
               loadThread();
             } else
               report(code, data);
           });
}
void Bridge::confirmSend(bool accepted) {
  auto captured = m_pendingSend;
  if (captured.isEmpty())
    return;
  if (accepted)
    deliver(captured, true);
  else {
    m_pendingSend = {};
    m_sending = false;
    emit busyChanged();
  }
}
void Bridge::reveal(const QString &id) {
  QString pub = m_current;
  int generation = m_generation;
  QJsonObject original;
  for (auto v : m_thread)
    if (text(v.toObject(), "id") == id)
      original = v.toObject();
  if (original.isEmpty())
    return;
  api.post("/api/reveal", {{"c", pub}, {"id", id}},
           [this, id, pub, generation, original](int code, QJsonObject data) mutable {
             if (code != 200) {
               report(code, data);
               return;
             }
             if (pub != m_current || generation != m_generation)
               return;
             original["hidden"] = false;
             original["body"] = data.value("body");
             m_revealed[id] = original;
             updateMessages();
           });
}
void Bridge::requestAction(const QString &action, const QString &pub, const QString &id) {
  QString target = pub.isEmpty() ? m_current : pub;
  m_pendingAction = {{"action", action}, {"pub", target}, {"id", id}};
  QString label = text(findContact(target), "label");
  if (action == "delete")
    emit confirmation("Delete this message?",
                      "This removes the message and attachment from this computer and cancels "
                      "queued retries. A copy already delivered remains on the other device.",
                      "Delete message", true);
  else if (action == "remove")
    emit confirmation(
        "Remove " + label + "?",
        "This deletes your conversation history with them from this computer. It can’t be undone.",
        "Remove and delete", true);
  else if (action == "block")
    emit confirmation("Block " + label + "?",
                      "They won’t be able to send you anything. You can unblock them later.",
                      "Block", true);
}
void Bridge::confirmAction(bool accepted) {
  auto action = m_pendingAction;
  m_pendingAction = {};
  if (!accepted)
    return;
  QString pub = text(action, "pub"), id = text(action, "id");
  if (text(action, "action") == "delete") {
    if (pub == m_current && m_revealed.contains(id)) {
      m_revealed.remove(id);
      for (int i = m_thread.size() - 1; i >= 0; i--)
        if (text(m_thread[i].toObject(), "id") == id)
          m_thread.removeAt(i);
      updateMessages();
      emit toast("Message deleted from this computer");
      return;
    }
    api.post("/api/message/delete", {{"c", pub}, {"id", id}},
             [this, pub, id](int code, QJsonObject data) {
               if (code == 200) {
                 if (pub == m_current) {
                   m_threadRequest++;
                   for (int i = m_thread.size() - 1; i >= 0; i--)
                     if (text(m_thread[i].toObject(), "id") == id)
                       m_thread.removeAt(i);
                   m_revealed.remove(id);
                   updateMessages();
                 }
                 emit toast("Message deleted from this computer");
                 refresh();
               } else
                 report(code, data);
             });
  } else
    contactAction(pub, text(action, "action"));
}
void Bridge::deleteMessage(const QString &id) {
  requestAction("delete", m_current, id);
}
void Bridge::contactAction(const QString &pub, const QString &action, const QString &label) {
  api.post("/api/contact", {{"c", pub}, {"action", action}, {"label", label}},
           [this, pub, action](int code, QJsonObject data) {
             if (code >= 200 && code < 300) {
               if ((action == "remove" || action == "decline") && pub == m_current)
                 back();
               emit toast("Contact updated");
               refresh();
             } else
               report(code, data);
           });
}
void Bridge::showDetail(const QString &key) {
  m_detail = key;
  initializeEditor();
  emit detailChanged();
  emit peopleChanged();
  emit detailSelected();
}
void Bridge::viewContact(const QString &pub) {
  setMode("contacts");
  showDetail(pub);
}
void Bridge::initializeEditor() {
  auto c = m_detail == "self" ? m_state.value("me").toObject() : findContact(m_detail);
  m_editLabel = text(c, m_detail == "self" ? "name" : "label");
  m_editAddress = text(c, "address");
  m_editNotes = text(c, "notes");
  m_editing = false;
  m_editorRevision++;
  emit editorChanged();
}
void Bridge::setEditLabel(const QString &v) {
  if (v != m_editLabel) {
    m_editLabel = v;
    m_editing = true;
    m_editorRevision++;
    emit editorChanged();
  }
}
void Bridge::setEditAddress(const QString &v) {
  if (v != m_editAddress) {
    m_editAddress = v;
    m_editing = true;
    m_editorRevision++;
    emit editorChanged();
  }
}
void Bridge::setEditNotes(const QString &v) {
  if (v != m_editNotes) {
    m_editNotes = v;
    m_editing = true;
    m_editorRevision++;
    emit editorChanged();
  }
}
void Bridge::saveContact() {
  if (m_saving)
    return;
  QString key = m_detail;
  int revision = m_editorRevision;
  bool self = key == "self";
  QJsonObject body = self ? m_state.value("settings").toObject()
                          : QJsonObject{{"c", key},
                                        {"action", "edit"},
                                        {"label", m_editLabel},
                                        {"address", m_editAddress},
                                        {"notes", m_editNotes}};
  if (self)
    body["name"] = m_editLabel;
  m_saving = true;
  emit busyChanged();
  api.post(self ? "/api/settings" : "/api/contact", body,
           [this, key, revision](int code, QJsonObject data) {
             m_saving = false;
             emit busyChanged();
             if (code >= 200 && code < 300) {
               if (m_mode == "contacts" && key == m_detail && revision == m_editorRevision) {
                 m_editing = false;
                 emit editorChanged();
               }
               emit toast("Saved");
               refresh();
             } else
               report(code, data);
           });
}
void Bridge::saveSettings(const QString &name, int days, bool secret) {
  if (m_saving)
    return;
  m_saving = true;
  emit busyChanged();
  api.post("/api/settings",
           {{"name", name}, {"retention_days", days}, {"secret_detection", secret}},
           [this](int code, QJsonObject data) {
             m_saving = false;
             emit busyChanged();
             if (code == 200) {
               emit settingsSaved();
               emit toast("Settings saved");
               refresh();
             } else
               report(code, data);
           });
}
void Bridge::beginAdd() {
  m_found = {};
  emit dialogChanged();
  emit openDialog("add");
}
void Bridge::showFound(const QVariantMap &value) {
  m_found = QJsonObject::fromVariantMap(value);
  emit dialogChanged();
}
void Bridge::lookup(const QString &target) {
  if (m_lookingUp)
    return;
  if (target.trimmed().isEmpty()) {
    emit toast("Paste a code or address first.", true);
    return;
  }
  m_lookingUp = true;
  emit dialogChanged();
  api.post("/api/lookup", {{"target", target.trimmed()}}, [this](int code, QJsonObject data) {
    m_lookingUp = false;
    if (code == 200)
      m_found = data;
    else
      report(code, data);
    emit dialogChanged();
  });
}
void Bridge::addFound(const QString &label) {
  auto found = m_found;
  QString pub = text(found, "pub");
  if (pub.isEmpty())
    return;
  auto success = [this, pub](int code, QJsonObject data) {
    if (code >= 200 && code < 300) {
      emit closeDialog();
      emit toast(
          "Contact updated. Compare their identity words before sharing private information.");
      api.get("/api/state", [this, pub](int code, QJsonObject state) {
        if (code == 200) {
          m_state = state;
          emit stateChanged();
          emit peopleChanged();
          select(pub);
        }
      });
    } else
      report(code, data);
  };
  if (found.value("known").toBool()) {
    QString status = text(found, "status");
    if (status == "pending" || status == "blocked")
      api.post("/api/contact", {{"c", pub}, {"action", status == "pending" ? "accept" : "unblock"}},
               success);
    else {
      emit closeDialog();
      select(pub);
    }
  } else
    api.post("/api/add",
             {{"pub", pub},
              {"name", found.value("name")},
              {"addr", found.value("addr")},
              {"label", label}},
             success);
}
void Bridge::verify() {
  QString pub = m_current;
  api.get("/api/safety?c=" + QString::fromUtf8(QUrl::toPercentEncoding(pub)),
          [this, pub](int code, QJsonObject data) {
            if (code == 200 && pub == m_current) {
              m_safetyCode = text(data, "code");
              emit dialogChanged();
              emit openDialog("verify");
            } else if (code != 200)
              report(code, data);
          });
}
void Bridge::loadNetwork() {
  int request = ++m_networkRequest;
  api.get("/api/network", [this, request](int code, QJsonObject data) {
    if (request != m_networkRequest)
      return;
    if (code != 200) {
      report(code, data);
      return;
    }
    QJsonObject previous;
    for (auto value : m_network.value("devices").toArray())
      if (text(value.toObject(), "id") == m_detail)
        previous = value.toObject();
    bool found = false;
    for (auto value : data.value("devices").toArray())
      if (text(value.toObject(), "id") == m_detail)
        found = true;
    if (!previous.isEmpty() && !found)
      for (auto value : data.value("devices").toArray()) {
        auto d = value.toObject();
        bool same = d.value("aliases").toArray().contains(m_detail) ||
                    (!text(previous, "pub").isEmpty() && text(previous, "pub") == text(d, "pub"));
        for (auto mac : previous.value("macs").toArray())
          if (d.value("macs").toArray().contains(mac))
            same = true;
        if (same) {
          m_detail = text(d, "id");
          break;
        }
      }
    if (data != m_network) {
      m_network = data;
      emit networkChanged();
      emit detailChanged();
      emit peopleChanged();
    }
  });
}
void Bridge::scan() {
  api.post("/api/network/scan", {}, [this](int code, QJsonObject data) {
    if (code >= 200 && code < 300) {
      emit toast("Scanning local devices");
      loadNetwork();
    } else
      report(code, data);
  });
}
void Bridge::preferences() {
  event({{"event", "preferences"},
         {"theme", m_theme},
         {"fontSize", m_fontSize},
         {"notifications", m_notifications},
         {"closeToTray", m_closeToTray}});
}
void Bridge::setTheme(const QString &v) {
  if (QStringList{"light", "dark", "system"}.contains(v) && m_theme != v) {
    m_theme = v;
    applyNativePalette(m_theme == "dark" || (m_theme == "system" && m_systemDark));
    emit appearanceChanged();
    preferences();
  }
}
void Bridge::setFontSize(int v) {
  v = qBound(80, v, 160);
  if (m_fontSize != v) {
    m_fontSize = v;
    emit appearanceChanged();
    preferences();
  }
}
void Bridge::setNotifications(bool v) {
  if (m_notifications != v) {
    m_notifications = v;
    emit appearanceChanged();
    preferences();
  }
}
void Bridge::setCloseToTray(bool v) {
  if (m_closeToTray != v) {
    m_closeToTray = v;
    emit appearanceChanged();
    preferences();
  }
}
void Bridge::copy(const QString &v) {
  QApplication::clipboard()->setText(v);
  emit toast("Copied");
}
void Bridge::openLink(const QUrl &url) {
  if ((url.scheme() == "https" || url.scheme() == "http") && !url.host().isEmpty() &&
      url.userInfo().isEmpty())
    QDesktopServices::openUrl(url);
}
void Bridge::report(int code, const QJsonObject &data, const QString &success) {
  if (code >= 200 && code < 300) {
    if (!success.isEmpty())
      emit toast(success);
  } else
    emit toast(data.value("error").toString("Something went wrong. Try again."), true);
}
void Bridge::event(const QJsonObject &data) {
  auto line = QJsonDocument(data).toJson(QJsonDocument::Compact) + '\n';
  fwrite(line.constData(), 1, size_t(line.size()), stdout);
  fflush(stdout);
}
QJsonObject Bridge::session(int width, int height) const {
  QJsonArray drafts;
  auto saved = m_drafts;
  if (!m_current.isEmpty())
    saved[m_current] = m_draft;
  for (auto it = saved.begin(); it != saved.end(); it++)
    drafts << QJsonArray{it.key(), it.value()};
  QJsonObject data = {{"theme", m_theme},
                      {"fontSize", m_fontSize},
                      {"notifications", m_notifications},
                      {"closeToTray", m_closeToTray},
                      {"width", width},
                      {"height", height},
                      {"panelMode", m_mode},
                      {"current", m_current},
                      {"detailSelection", m_detail},
                      {"sidebarSearch", m_sidebarSearch},
                      {"drafts", drafts},
                      {"burn", m_burn}};
  if (m_editing)
    data["editing"] =
        QJsonObject{{"label", m_editLabel}, {"address", m_editAddress}, {"notes", m_editNotes}};
  return data;
}
bool Bridge::closeWindow(int width, int height) {
  if (busy()) {
    emit toast("Wait for the current send, save, or file transfer to finish before closing.");
    return false;
  }
  event({{"event", "closed"}, {"session", session(width, height)}});
  QTimer::singleShot(0, qApp, &QCoreApplication::quit);
  return true;
}
QString Bridge::initials(const QString &name) const {
  auto words = name.trimmed().split(QRegularExpression("\\s+"), Qt::SkipEmptyParts);
  if (words.isEmpty())
    return "?";
  QString first = words.first().left(words.first().front().isHighSurrogate() ? 2 : 1);
  if (words.size() > 1)
    first += words.last().left(words.last().front().isHighSurrogate() ? 2 : 1);
  return first.toUpper();
}
QColor Bridge::avatarColor(const QString &name, const QString &pub) const {
  quint32 hash = 0;
  for (uint c : (name + pub).toUcs4())
    hash = hash * 31 + c;
  qint32 signedHash = qint32(hash);
  quint32 index = signedHash < 0 ? quint32(-qint64(signedHash)) : quint32(signedHash);
  static const char *colors[] = {"#0b5cad", "#0b5cad", "#344772", "#536789",
                                 "#465162", "#384f91", "#5a6480", "#647084"};
  return QColor(colors[index % 8]);
}
QString Bridge::timeText(double ts) const {
  return QLocale().toString(QDateTime::fromSecsSinceEpoch(qint64(ts)).time(), "h:mm AP");
}
QString Bridge::dateText(double ts) const {
  auto date = QDateTime::fromSecsSinceEpoch(qint64(ts)).date();
  if (date == QDate::currentDate())
    return "Today";
  if (date == QDate::currentDate().addDays(-1))
    return "Yesterday";
  return QLocale().toString(date, date.year() == QDate::currentDate().year() ? "ddd, MMM d"
                                                                             : "ddd, MMM d, yyyy");
}
QString Bridge::lastSeen(double ts) const {
  if (ts <= 0)
    return "never";
  qint64 seconds = qMax<qint64>(0, QDateTime::currentSecsSinceEpoch() - qint64(ts));
  if (seconds < 60)
    return "just now";
  if (seconds < 3600)
    return QString::number(seconds / 60) + " min ago";
  if (seconds < 172800)
    return QString::number(seconds / 3600) + " h ago";
  return QString::number(seconds / 86400) + " days ago";
}
QString Bridge::formatSize(double size) const {
  if (size < 1024)
    return QString::number(size) + " B";
  if (size < 1048576)
    return QString::number(size / 1024, 'f', 1) + " KiB";
  return QString::number(size / 1048576, 'f', 1) + " MiB";
}
QString Bridge::richText(const QString &value) const {
  QString result;
  int offset = 0;
  QRegularExpression pattern("https?://[^\\s<>]+", QRegularExpression::CaseInsensitiveOption);
  auto matches = pattern.globalMatch(value);
  while (matches.hasNext()) {
    auto match = matches.next();
    result += value.mid(offset, match.capturedStart() - offset).toHtmlEscaped();
    QString link = match.captured();
    while (!link.isEmpty() && QString(".,;:!?)").contains(link.back()))
      link.chop(1);
    QUrl url(link);
    QString escaped = link.toHtmlEscaped();
    if (url.isValid() && !url.host().isEmpty() && url.userInfo().isEmpty())
      result += "<a href=\"" + escaped + "\">" + escaped + "</a>";
    else
      result += escaped;
    result += match.captured().mid(link.size()).toHtmlEscaped();
    offset = match.capturedEnd();
  }
  result += value.mid(offset).toHtmlEscaped();
  result.replace('\n', "<br/>");
  return result;
}
double Bridge::textWidth(const QString &value, double size) const {
  QFont font("Noto Sans");
  font.setPixelSize(qRound(size));
  QFontMetricsF metrics(font);
  double width = 0;
  for (const QString &line : value.split('\n'))
    width = qMax(width, metrics.horizontalAdvance(line));
  return width;
}
static QVariantMap field(QString label, QString value) {
  return {{"label", label}, {"value", value.isEmpty() ? QString("Unknown") : value}};
}
static QVariantMap card(QString title, QVariantList fields) {
  return {{"title", title}, {"fields", fields}};
}
QString Bridge::detailTitle() const {
  if (m_mode == "contacts") {
    auto c = m_detail == "self" ? m_state.value("me").toObject() : findContact(m_detail);
    if (c.isEmpty())
      return "Contact book";
    return text(c, m_detail == "self" ? "name" : "label") + (m_detail == "self" ? " (You)" : "");
  }
  if (m_detail == "lan")
    return "LAN information";
  for (auto value : m_network.value("devices").toArray()) {
    auto d = value.toObject();
    if (text(d, "id") == m_detail) {
      auto name = text(d, "name");
      return name.isEmpty() ? text(d, "ip") : name;
    }
  }
  return "Device no longer visible";
}
QString Bridge::detailSubtitle() const {
  if (m_mode == "contacts")
    return m_detail == "self" ? "Your profile and broadcast identity"
                              : "Your assigned name is separate from their broadcast name";
  if (m_detail == "lan")
    return text(m_network, "coverage");
  for (auto value : m_network.value("devices").toArray())
    if (text(value.toObject(), "id") == m_detail)
      return text(value.toObject(), "source");
  return "";
}
QVariantList Bridge::cards() const {
  QVariantList result;
  if (m_mode == "contacts") {
    bool self = m_detail == "self";
    auto c = self ? m_state.value("me").toObject() : findContact(m_detail);
    if (c.isEmpty())
      return result;
    QVariantList fields = {
        field("Broadcast name", text(c, self ? "name" : "their_name")),
        field("Device", text(c, "device")),
        field("Platform", text(c, "platform")),
        field("Last online", self ? "Online now" : fullTime(c.value("last_seen"))),
        field("Status", self ? "This is you" : text(c, "status")),
        field("Identity words", text(c, "fingerprint")),
        field("Public key", text(c, "pub"))};
    if (self) {
      fields << field("Addresses", joined(c.value("addresses")))
             << field("Invite code", text(c, "invite"));
    } else
      fields << field("First seen", fullTime(c.value("first_seen")));
    result << card("Contact information", fields);
    return result;
  }
  if (m_detail == "lan") {
    QString detection = m_network.value("scanning").toBool()      ? "Scanning local addresses…"
                        : m_network.value("identifying").toBool() ? "Identifying devices…"
                                                                  : "Up to date";
    auto lan =
        card("Your network",
             {field("Computer", text(m_network, "hostname")),
              field("Platform", text(m_network, "platform")),
              field("Gateway", text(m_network, "gateway")),
              field("DNS resolver", joined(m_network.value("dns"))),
              field("Messaging port", QString::number(m_state.value("port").toInt())),
              field("Devices found", QString::number(m_network.value("devices").toArray().size())),
              field("Last full scan", fullTime(m_network.value("last_scan"))),
              field("Last identification", fullTime(m_network.value("last_identified"))),
              field("Detection", detection)});
    lan["scan"] = true;
    result << lan;
    for (auto value : m_network.value("interfaces").toArray()) {
      auto i = value.toObject();
      QVariantList fields = {
          field("Addresses / subnets", joined(i.value("addresses"))), field("MAC", text(i, "mac")),
          field("MTU", QString::number(i.value("mtu").toInt())),
          field("Connection", i.value("virtual").toBool()
                                  ? "Virtual interface (excluded from LAN probes)"
                                  : "Local network")};
      if (i.value("default_route").toBool())
        fields << field("Role", "Default route");
      result << card(text(i, "name") + (i.value("up").toBool() ? " · connected" : " · down"),
                     fields);
    }
  } else {
    QJsonObject d;
    for (auto value : m_network.value("devices").toArray())
      if (text(value.toObject(), "id") == m_detail)
        d = value.toObject();
    if (d.isEmpty()) {
      result << QVariantMap{
          {"title", "This device is no longer in the current scan"},
          {"description",
           "It may be offline, or its details may have been merged into another device."},
          {"missing", true},
          {"fields", QVariantList{}}};
      return result;
    }
    QVariantList fields = {field("Device type", text(d, "kind")),
                           field("Identification", text(d, "identification")),
                           field("Confidence", text(d, "confidence"))};
    for (auto pair : QList<QPair<QString, QString>>{{"hostname", "Hostname"},
                                                    {"broadcast_name", "Broadcast name"}})
      if (!d.value(pair.first).toString().isEmpty())
        fields << field(pair.second, d.value(pair.first).toString());
    fields << field("IP addresses", d.value("addresses").toArray().isEmpty()
                                        ? text(d, "ip")
                                        : joined(d.value("addresses")));
    if (!d.value("macs").toArray().isEmpty() || !text(d, "mac").isEmpty())
      fields << field("MAC addresses", d.value("macs").toArray().isEmpty()
                                           ? text(d, "mac")
                                           : joined(d.value("macs")));
    fields << field("Interfaces", d.value("interfaces").toArray().isEmpty()
                                      ? text(d, "interface")
                                      : joined(d.value("interfaces")));
    for (auto pair : QList<QPair<QString, QString>>{{"vendor", "Adapter vendor"},
                                                    {"manufacturer", "Manufacturer (advertised)"},
                                                    {"model", "Model (advertised)"},
                                                    {"platform", "Platform"}})
      if (!d.value(pair.first).toString().isEmpty())
        fields << field(pair.second, d.value(pair.first).toString());
    fields << field("Status", text(d, "status"))
           << field("Last observed", d.value("seen").toDouble() > 0 ? fullTime(d.value("seen"))
                                                                    : QString("Not confirmed yet"));
    auto device = card("Device information", fields);
    if (!text(d, "pub").isEmpty() && !d.value("self").toBool()) {
      device["contactPub"] = text(d, "pub");
      device["known"] = !findContact(text(d, "pub")).isEmpty();
      device["device"] = d.toVariantMap();
    }
    result << device;
    if (!d.value("services").toArray().isEmpty())
      result << card("Detected services", {field("Services", joined(d.value("services"))),
                                           field("Sources", joined(d.value("sources")))});
  }
  return result;
}
QUrl Bridge::suggestedFile(const QString &name) const {
  QString folder = QStandardPaths::writableLocation(QStandardPaths::DownloadLocation);
  if (folder.isEmpty())
    folder = QDir::homePath();
  QString safe = QFileInfo(name).fileName();
  if (safe.isEmpty() || safe == "." || safe == "..")
    safe = "attachment";
  return QUrl::fromLocalFile(QDir(folder).filePath(safe));
}
void Bridge::upload(const QUrl &url) {
  if (!url.isLocalFile() || m_current.isEmpty())
    return;
  QString pub = m_current;
  if (text(findContact(pub), "status") != "accepted") {
    emit toast("This contact must accept your request before you can share files.", true);
    return;
  }
  auto file = new QFile(url.toLocalFile());
  if (!file->open(QIODevice::ReadOnly)) {
    emit toast("Could not open that file.", true);
    delete file;
    return;
  }
  if (file->size() > 256LL * 1048576) {
    emit toast("Files must be 256 MiB or smaller.", true);
    delete file;
    return;
  }
  QString path = "/api/files?c=" + QString::fromUtf8(QUrl::toPercentEncoding(pub)) + "&name=" +
                 QString::fromUtf8(QUrl::toPercentEncoding(QFileInfo(file->fileName()).fileName()));
  auto request = api.request(path);
  request.setHeader(QNetworkRequest::ContentTypeHeader, "application/octet-stream");
  request.setHeader(QNetworkRequest::ContentLengthHeader, file->size());
  auto reply = api.manager.post(request, file);
  file->setParent(reply);
  m_transfers++;
  m_transferText = "Sending file…";
  emit busyChanged();
  QTimer::singleShot(600000, reply, [reply] {
    if (!reply->isFinished())
      reply->abort();
  });
  connect(reply, &QNetworkReply::uploadProgress, this, [this](qint64 sent, qint64 total) {
    m_transferText =
        total > 0 ? "Sending file: " + QString::number(sent * 100 / total) + "%" : "Sending file…";
    emit busyChanged();
  });
  connect(reply, &QNetworkReply::finished, this, [this, reply] {
    m_transfers--;
    m_transferText.clear();
    emit busyChanged();
    int code = reply->attribute(QNetworkRequest::HttpStatusCodeAttribute).toInt();
    auto data = QJsonDocument::fromJson(reply->readAll()).object();
    report(code, data, "File queued for delivery");
    reply->deleteLater();
    refresh();
    loadThread();
  });
}
void Bridge::download(const QString &id, const QUrl &url) {
  if (!url.isLocalFile() || m_current.isEmpty())
    return;
  QJsonObject metadata;
  for (auto v : m_thread)
    if (text(v.toObject(), "id") == id)
      metadata = v.toObject().value("file").toObject();
  if (metadata.isEmpty()) {
    emit toast("That file is no longer available.", true);
    return;
  }
  auto output = std::make_shared<QSaveFile>(url.toLocalFile());
  if (!output->open(QIODevice::WriteOnly)) {
    emit toast("Could not save to that location.", true);
    return;
  }
  auto hash = std::make_shared<QCryptographicHash>(QCryptographicHash::Sha256);
  auto bytes = std::make_shared<qint64>(0);
  auto failed = std::make_shared<bool>(false);
  auto reply = api.manager.get(
      api.request("/api/file?c=" + QString::fromUtf8(QUrl::toPercentEncoding(m_current)) +
                  "&id=" + QString::fromUtf8(QUrl::toPercentEncoding(id))));
  m_transfers++;
  m_transferText = "Saving file…";
  emit busyChanged();
  auto consume = [reply, output, hash, bytes, failed] {
    auto data = reply->readAll();
    *bytes += data.size();
    if (*bytes > 256LL * 1048576 || output->write(data) != data.size()) {
      *failed = true;
      reply->abort();
    }
    hash->addData(data);
  };
  connect(reply, &QNetworkReply::readyRead, reply, consume);
  QTimer::singleShot(600000, reply, [reply] {
    if (!reply->isFinished())
      reply->abort();
  });
  connect(reply, &QNetworkReply::finished, this,
          [this, reply, output, hash, bytes, failed, metadata, consume] {
            consume();
            bool ok = reply->error() == QNetworkReply::NoError &&
                      reply->attribute(QNetworkRequest::HttpStatusCodeAttribute).toInt() == 200 &&
                      !*failed;
            ok = ok && *bytes == qint64(metadata.value("size").toDouble()) &&
                 QString::fromLatin1(hash->result().toHex())
                         .compare(text(metadata, "sha256"), Qt::CaseInsensitive) == 0;
            if (ok)
              ok = output->commit();
            else
              output->cancelWriting();
            m_transfers--;
            m_transferText.clear();
            emit busyChanged();
            emit toast(
                ok ? "File saved" : "Could not save the file, or its integrity check failed.", !ok);
            reply->deleteLater();
          });
}
void Bridge::portalChanged(const QString &group, const QString &key, const QDBusVariant &value) {
  if (group == "org.freedesktop.appearance" && key == "color-scheme") {
    m_systemDark = value.variant().toUInt() == 1;
    applyNativePalette(m_theme == "dark" || (m_theme == "system" && m_systemDark));
    emit appearanceChanged();
  }
}
