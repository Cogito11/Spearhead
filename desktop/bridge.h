#pragma once
#include "api_client.h"
#include <QAbstractListModel>
#include <QDBusVariant>
#include <QtCore>
#include <QtGui>
#include <QtNetwork>

class MessageModel : public QAbstractListModel {
  Q_OBJECT
public:
  QList<QVariantMap> rows;
  explicit MessageModel(QObject *parent = nullptr) : QAbstractListModel(parent) {}
  int rowCount(const QModelIndex &parent = {}) const override {
    return parent.isValid() ? 0 : rows.size();
  }
  QVariant data(const QModelIndex &index, int role) const override;
  QHash<int, QByteArray> roleNames() const override {
    return {{Qt::UserRole + 1, "item"}};
  }
  void update(const QList<QVariantMap> &next);
};

class Bridge : public QObject {
  Q_OBJECT
  Q_PROPERTY(QString version READ version CONSTANT)
  Q_PROPERTY(QVariantMap state READ state NOTIFY stateChanged)
  Q_PROPERTY(QVariantMap network READ network NOTIFY networkChanged)
  Q_PROPERTY(QVariantList people READ people NOTIFY peopleChanged)
  Q_PROPERTY(MessageModel *peopleModel READ peopleModel CONSTANT)
  Q_PROPERTY(QVariantList requests READ requests NOTIFY stateChanged)
  Q_PROPERTY(QVariantMap contact READ contact NOTIFY contactChanged)
  Q_PROPERTY(QVariantList cards READ cards NOTIFY detailChanged)
  Q_PROPERTY(QString detailTitle READ detailTitle NOTIFY detailChanged)
  Q_PROPERTY(QString detailSubtitle READ detailSubtitle NOTIFY detailChanged)
  Q_PROPERTY(QString mode READ mode WRITE setMode NOTIFY navigationChanged)
  Q_PROPERTY(QString current READ current NOTIFY navigationChanged)
  Q_PROPERTY(QString detail READ detail NOTIFY detailChanged)
  Q_PROPERTY(QString sidebarSearch READ sidebarSearch WRITE setSidebarSearch NOTIFY peopleChanged)
  Q_PROPERTY(QString search READ search WRITE setSearch NOTIFY searchChanged)
  Q_PROPERTY(bool searchOpen READ searchOpen WRITE setSearchOpen NOTIFY searchChanged)
  Q_PROPERTY(QString draft READ draft WRITE setDraft NOTIFY draftChanged)
  Q_PROPERTY(bool burn READ burn WRITE setBurn NOTIFY draftChanged)
  Q_PROPERTY(bool sending READ sending NOTIFY busyChanged)
  Q_PROPERTY(bool busy READ busy NOTIFY busyChanged)
  Q_PROPERTY(bool connected READ connected NOTIFY stateChanged)
  Q_PROPERTY(QString theme READ theme WRITE setTheme NOTIFY appearanceChanged)
  Q_PROPERTY(int fontSize READ fontSize WRITE setFontSize NOTIFY appearanceChanged)
  Q_PROPERTY(bool systemDark READ systemDark NOTIFY appearanceChanged)
  Q_PROPERTY(bool notifications READ notifications WRITE setNotifications NOTIFY appearanceChanged)
  Q_PROPERTY(bool closeToTray READ closeToTray WRITE setCloseToTray NOTIFY appearanceChanged)
  Q_PROPERTY(QString editLabel READ editLabel WRITE setEditLabel NOTIFY editorChanged)
  Q_PROPERTY(QString editAddress READ editAddress WRITE setEditAddress NOTIFY editorChanged)
  Q_PROPERTY(QString editNotes READ editNotes WRITE setEditNotes NOTIFY editorChanged)
  Q_PROPERTY(bool editing READ editing NOTIFY editorChanged)
  Q_PROPERTY(bool saving READ saving NOTIFY busyChanged)
  Q_PROPERTY(QVariantMap found READ found NOTIFY dialogChanged)
  Q_PROPERTY(bool lookingUp READ lookingUp NOTIFY dialogChanged)
  Q_PROPERTY(QString safetyCode READ safetyCode NOTIFY dialogChanged)
  Q_PROPERTY(QString transferText READ transferText NOTIFY busyChanged)
  Q_PROPERTY(MessageModel *messages READ messages CONSTANT)
public:
  Api api;
  explicit Bridge(const QJsonObject &payload, QObject *parent = nullptr);
  QString version() const {
    return QCoreApplication::applicationVersion();
  }
  QVariantMap state() const {
    return m_state.toVariantMap();
  }
  QVariantMap network() const {
    return m_network.toVariantMap();
  }
  QVariantList people() const;
  QVariantList requests() const {
    return m_state.value("requests").toArray().toVariantList();
  }
  QVariantMap contact() const;
  QVariantList cards() const;
  QString detailTitle() const;
  QString detailSubtitle() const;
  QString mode() const {
    return m_mode;
  }
  QString current() const {
    return m_current;
  }
  QString detail() const {
    return m_detail;
  }
  QString sidebarSearch() const {
    return m_sidebarSearch;
  }
  QString search() const {
    return m_search;
  }
  bool searchOpen() const {
    return m_searchOpen;
  }
  QString draft() const {
    return m_draft;
  }
  bool burn() const {
    return m_burn;
  }
  bool sending() const {
    return m_sending;
  }
  bool busy() const {
    return m_sending || m_transfers > 0 || m_saving;
  }
  bool connected() const {
    return m_connected;
  }
  QString theme() const {
    return m_theme;
  }
  int fontSize() const {
    return m_fontSize;
  }
  bool systemDark() const {
    return m_systemDark;
  }
  bool notifications() const {
    return m_notifications;
  }
  bool closeToTray() const {
    return m_closeToTray;
  }
  QString editLabel() const {
    return m_editLabel;
  }
  QString editAddress() const {
    return m_editAddress;
  }
  QString editNotes() const {
    return m_editNotes;
  }
  bool editing() const {
    return m_editing;
  }
  bool saving() const {
    return m_saving;
  }
  QVariantMap found() const {
    return m_found.toVariantMap();
  }
  bool lookingUp() const {
    return m_lookingUp;
  }
  QString safetyCode() const {
    return m_safetyCode;
  }
  QString transferText() const {
    return m_transferText;
  }
  MessageModel *messages() {
    return &m_messages;
  }
  MessageModel *peopleModel() {
    return &m_people;
  }
  void start();
  void setMode(const QString &value);
  void setSidebarSearch(const QString &value);
  void setSearch(const QString &value);
  void setSearchOpen(bool value);
  void setDraft(const QString &value);
  void setBurn(bool value);
  void setTheme(const QString &value);
  void setFontSize(int value);
  void setNotifications(bool value);
  void setCloseToTray(bool value);
  void setEditLabel(const QString &value);
  void setEditAddress(const QString &value);
  void setEditNotes(const QString &value);
  Q_INVOKABLE void refresh();
  Q_INVOKABLE void select(const QString &pub);
  Q_INVOKABLE void back();
  Q_INVOKABLE void showDetail(const QString &key);
  Q_INVOKABLE void viewContact(const QString &pub);
  Q_INVOKABLE void send();
  Q_INVOKABLE void confirmSend(bool accepted);
  Q_INVOKABLE void reveal(const QString &id);
  Q_INVOKABLE void deleteMessage(const QString &id);
  Q_INVOKABLE void contactAction(const QString &pub, const QString &action,
                                 const QString &label = {});
  Q_INVOKABLE void requestAction(const QString &action, const QString &pub = {},
                                 const QString &id = {});
  Q_INVOKABLE void confirmAction(bool accepted);
  Q_INVOKABLE void saveContact();
  Q_INVOKABLE void saveSettings(const QString &name, int days, bool secret);
  Q_INVOKABLE void beginAdd();
  Q_INVOKABLE void lookup(const QString &target);
  Q_INVOKABLE void showFound(const QVariantMap &value);
  Q_INVOKABLE void addFound(const QString &label);
  Q_INVOKABLE void verify();
  Q_INVOKABLE void scan();
  Q_INVOKABLE void upload(const QUrl &file);
  Q_INVOKABLE void download(const QString &id, const QUrl &file);
  Q_INVOKABLE QUrl suggestedFile(const QString &name) const;
  Q_INVOKABLE void copy(const QString &text);
  Q_INVOKABLE void openLink(const QUrl &url);
  Q_INVOKABLE bool closeWindow(int width, int height);
  Q_INVOKABLE void setWindowActive(bool active) {
    m_windowActive = active;
  }
  Q_INVOKABLE QString initials(const QString &name) const;
  Q_INVOKABLE QColor avatarColor(const QString &name, const QString &pub) const;
  Q_INVOKABLE QString timeText(double timestamp) const;
  Q_INVOKABLE QString dateText(double timestamp) const;
  Q_INVOKABLE QString lastSeen(double timestamp) const;
  Q_INVOKABLE QString formatSize(double size) const;
  Q_INVOKABLE QString richText(const QString &text) const;
  Q_INVOKABLE double textWidth(const QString &text, double size) const;
  QJsonObject session(int width, int height) const;
  static void event(const QJsonObject &data);
signals:
  void stateChanged();
  void peopleChanged();
  void navigationChanged();
  void contactChanged();
  void detailSelected();
  void messagesChanging(bool reset);
  void messagesChanged(bool reset);
  void networkChanged();
  void detailChanged();
  void editorChanged();
  void draftChanged();
  void appearanceChanged();
  void busyChanged();
  void searchChanged();
  void dialogChanged();
  void toast(const QString &text, bool error = false);
  void openDialog(const QString &name);
  void confirmation(const QString &title, const QString &text, const QString &button, bool danger);
  void secrets(const QVariantList &findings);
  void settingsSaved();
  void contactSaved();
  void closeDialog();
private slots:
  void portalChanged(const QString &group, const QString &key, const QDBusVariant &value);

private:
  QJsonObject m_state, m_network, m_found, m_restore, m_pendingSend, m_pendingAction;
  QJsonArray m_thread;
  QMap<QString, QJsonObject> m_revealed;
  QMap<QString, QString> m_drafts;
  QMap<QString, int> m_unread;
  MessageModel m_messages, m_people;
  QTimer m_timer;
  QString m_mode = "conversations", m_current, m_detail = "self", m_sidebarSearch, m_search,
          m_draft, m_theme = "system";
  QString m_editLabel, m_editAddress, m_editNotes, m_safetyCode, m_transferText;
  bool m_searchOpen = false, m_burn = false, m_sending = false, m_connected = true,
       m_systemDark = false, m_notifications = true, m_closeToTray = true, m_editing = false,
       m_saving = false, m_lookingUp = false, m_refreshing = false, m_restored = false,
       m_windowActive = true;
  int m_fontSize = 100, m_generation = 0, m_threadRequest = 0, m_networkRequest = 0,
      m_editorRevision = 0, m_transfers = 0;
  void loadThread();
  void loadNetwork();
  void updateMessages(bool reset = false);
  void initializeEditor();
  void preferences();
  QJsonObject findContact(const QString &pub) const;
  void deliver(const QJsonObject &captured, bool force);
  void report(int code, const QJsonObject &data, const QString &success = {});
  void restore();
};
