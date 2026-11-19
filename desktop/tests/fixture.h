#pragma once
#include <QtCore>
#include <QtNetwork>
class Fixture : public QTcpServer {
public:
  QJsonObject state, network;
  QJsonArray messages;
  QByteArray file;
  QList<QPair<QString, QJsonObject>> posts;
  QList<QByteArray> authenticated;
  QByteArray uploaded;
  QPointer<QTcpSocket> heldSave;
  bool holdSave = false;
  int sendDelay = 0;
  QList<QPair<int, QJsonObject>> networkReplies;
  Fixture(QObject *parent = nullptr) : QTcpServer(parent) {
    QFile source(QString(SPEARHEAD_TEST_SOURCE_DIR) + "/tests/fixtures.json");
    source.open(QIODevice::ReadOnly);
    auto data = QJsonDocument::fromJson(source.readAll()).object();
    state = data.value("state").toObject();
    network = data.value("network").toObject();
    messages = data.value("messages").toArray();
    file = QByteArray::fromBase64(data.value("file").toString().toUtf8());
    listen(QHostAddress::LocalHost, 0);
    connect(this, &QTcpServer::newConnection, this, [this] {
      while (hasPendingConnections()) {
        auto socket = nextPendingConnection();
        connect(socket, &QTcpSocket::readyRead, this, [this, socket] {
          auto bytes = socket->property("input").toByteArray() + socket->readAll();
          socket->setProperty("input", bytes);
          int split = bytes.indexOf("\r\n\r\n");
          if (split < 0)
            return;
          auto headers = bytes.left(split);
          int length = 0;
          for (const auto &line : headers.split('\n'))
            if (line.toLower().startsWith("content-length:"))
              length = line.mid(15).trimmed().toInt();
          if (bytes.size() < split + 4 + length || socket->property("handled").toBool())
            return;
          socket->setProperty("handled", true);
          handle(socket, headers, bytes.mid(split + 4, length));
        });
        connect(socket, &QTcpSocket::disconnected, socket, &QObject::deleteLater);
      }
    });
  }
  QUrl url() const {
    return QUrl("http://127.0.0.1:" + QString::number(serverPort()) + "/?t=qt-test-secret");
  }
  static void response(QTcpSocket *socket, int code, QByteArray body,
                       const QByteArray &type = "application/json") {
    if (!socket || socket->state() == QAbstractSocket::UnconnectedState)
      return;
    socket->write("HTTP/1.1 " + QByteArray::number(code) + " OK\r\nContent-Type: " + type +
                  "\r\nContent-Length: " + QByteArray::number(body.size()) +
                  "\r\nConnection: close\r\n\r\n" + body);
    socket->disconnectFromHost();
  }
  static void json(QTcpSocket *socket, int code, const QJsonObject &data) {
    response(socket, code, QJsonDocument(data).toJson(QJsonDocument::Compact));
  }
  void releaseSave() {
    if (heldSave) {
      json(heldSave, 200, {{"ok", true}});
      heldSave.clear();
    }
  }
  void handle(QTcpSocket *socket, const QByteArray &headers, const QByteArray &body) {
    auto first = headers.split('\n').first().split(' ');
    QUrl request(QString::fromUtf8(first.value(1)));
    QString path = request.path();
    bool post = first.value(0) == "POST";
    auto lower = headers.toLower();
    if (!lower.contains("cookie: spearhead=qt-test-secret") || !lower.contains("x-spearhead: 1")) {
      json(socket, 403, {{"error", "Authentication missing"}});
      return;
    }
    authenticated << headers;
    auto data = QJsonDocument::fromJson(body).object();
    if (post)
      posts << qMakePair(path, data);
    if (path == "/api/state") {
      json(socket, 200, state);
      return;
    }
    if (path == "/api/network" && !post) {
      if (!networkReplies.isEmpty()) {
        auto reply = networkReplies.takeFirst();
        QPointer<QTcpSocket> guarded = socket;
        QTimer::singleShot(reply.first, this, [guarded, reply] {
          if (guarded)
            json(guarded, 200, reply.second);
        });
      } else
        json(socket, 200, network);
      return;
    }
    if (path == "/api/thread") {
      json(socket, 200, {{"messages", messages}});
      return;
    }
    if (path == "/api/activity") {
      json(socket, 200, {{"events", 0}});
      return;
    }
    if (path == "/api/safety") {
      json(socket, 200, {{"code", "alpha beta gamma delta epsilon zeta eta theta"}});
      return;
    }
    if (path == "/api/send") {
      if (data.value("body").toString().contains("password") && !data.value("force").toBool()) {
        json(socket, 409,
             {{"error", "secrets"},
              {"findings",
               QJsonArray{QJsonObject{{"rule", "Credential"}, {"sample", "[redacted]"}}}}});
        return;
      }
      auto message = QJsonObject{{"id", "sent-" + QString::number(posts.size())},
                                 {"body", data.value("body")},
                                 {"ts", 1700000060},
                                 {"dir", "out"},
                                 {"state", "delivered"}};
      messages << message;
      QPointer<QTcpSocket> guarded = socket;
      QTimer::singleShot(sendDelay, this, [guarded, message] {
        if (guarded)
          json(guarded, 200, message);
      });
      return;
    }
    if (path == "/api/contact") {
      auto list = state.value("contacts").toArray();
      QString pub = data.value("c").toString(), action = data.value("action").toString();
      for (int i = list.size() - 1; i >= 0; i--) {
        auto c = list[i].toObject();
        if (c.value("pub").toString() != pub)
          continue;
        if (action == "remove") {
          list.removeAt(i);
          continue;
        }
        if (action == "edit" || action == "rename")
          c["label"] = data.value("label");
        if (action == "edit") {
          c["notes"] = data.value("notes");
          c["address"] = data.value("address");
        }
        if (action == "block")
          c["status"] = "blocked";
        if (action == "unblock" || action == "accept")
          c["status"] = "accepted";
        if (action == "verify")
          c["verified"] = true;
        list[i] = c;
      }
      state["contacts"] = list;
      if (holdSave) {
        heldSave = socket;
        return;
      }
      json(socket, 200, {{"ok", true}});
      return;
    }
    if (path == "/api/settings") {
      auto me = state.value("me").toObject();
      me["name"] = data.value("name");
      state["me"] = me;
      state["settings"] = QJsonObject{{"retention_days", data.value("retention_days")},
                                      {"secret_detection", data.value("secret_detection")}};
      json(socket, 200, {{"ok", true}});
      return;
    }
    if (path == "/api/message/delete" || path == "/api/reveal") {
      for (int i = messages.size() - 1; i >= 0; i--)
        if (messages[i].toObject().value("id") == data.value("id"))
          messages.removeAt(i);
      json(socket, 200,
           path == "/api/reveal" ? QJsonObject{{"body", "one-time-secret"}}
                                 : QJsonObject{{"deleted", true}});
      return;
    }
    if (path == "/api/lookup") {
      json(socket, 200,
           {{"pub", "new-key"},
            {"name", "New peer"},
            {"addr", "192.168.1.4:7777"},
            {"fingerprint", "new peer words"},
            {"known", false}});
      return;
    }
    if (path == "/api/add") {
      auto list = state.value("contacts").toArray();
      list << QJsonObject{{"pub", data.value("pub")},
                          {"label", data.value("label")},
                          {"their_name", data.value("name")},
                          {"status", "accepted"},
                          {"address", data.value("addr")}};
      state["contacts"] = list;
      json(socket, 200, {{"pub", data.value("pub")}});
      return;
    }
    if (path == "/api/network/scan") {
      json(socket, 202, {{"started", true}});
      return;
    }
    if (path == "/api/files") {
      uploaded = body;
      json(socket, 202, {{"state", "queued"}});
      return;
    }
    if (path == "/api/file") {
      response(socket, 200, file, "application/octet-stream");
      return;
    }
    json(socket, 404, {{"error", "Unknown fixture endpoint"}});
  }
};
