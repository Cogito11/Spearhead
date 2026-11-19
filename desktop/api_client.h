#pragma once
#include <QtCore>
#include <QtNetwork>
#include <functional>
class Api : public QObject {
  Q_OBJECT
public:
  QNetworkAccessManager manager;
  QUrl base;
  QString token;
  using Done = std::function<void(int, QJsonObject)>;
  explicit Api(const QUrl &url, QObject *parent = nullptr);
  QNetworkRequest request(const QString &path) const;
  void json(const QString &path, const QJsonObject &body, bool post, Done done);
  void get(const QString &path, Done done) {
    json(path, {}, false, std::move(done));
  }
  void post(const QString &path, const QJsonObject &body, Done done) {
    json(path, body, true, std::move(done));
  }
};
