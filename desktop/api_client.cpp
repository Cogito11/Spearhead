#include "api_client.h"
Api::Api(const QUrl &url, QObject *parent)
    : QObject(parent), base(url), token(QUrlQuery(url).queryItemValue("t")) {
  base.setPath("/");
  base.setQuery(QString());
  manager.setProxy(QNetworkProxy::NoProxy);
}
QNetworkRequest Api::request(const QString &path) const {
  QNetworkRequest result(base.resolved(QUrl(path)));
  result.setRawHeader("Cookie", ("spearhead=" + token).toUtf8());
  result.setRawHeader("X-SpearHead", "1");
  result.setAttribute(QNetworkRequest::RedirectPolicyAttribute,
                      QNetworkRequest::ManualRedirectPolicy);
  return result;
}
void Api::json(const QString &path, const QJsonObject &body, bool post, Done done) {
  auto request = this->request(path);
  request.setHeader(QNetworkRequest::ContentTypeHeader, "application/json");
  auto reply = post ? manager.post(request, QJsonDocument(body).toJson(QJsonDocument::Compact))
                    : manager.get(request);
  QTimer::singleShot(30000, reply, [reply] {
    if (!reply->isFinished())
      reply->abort();
  });
  connect(reply, &QNetworkReply::finished, reply, [reply, done] {
    int code = reply->attribute(QNetworkRequest::HttpStatusCodeAttribute).toInt();
    auto data = QJsonDocument::fromJson(reply->readAll()).object();
    if (!code)
      data["error"] = "The local service did not respond. Reopen SpearHead.";
    done(code, data);
    reply->deleteLater();
  });
}
