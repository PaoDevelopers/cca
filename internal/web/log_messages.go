package web

const (
	logMsgHTTPRequestStart            = "http.request.start"
	logMsgHTTPResponseJSON            = "http.response.json"
	logMsgHTTPResponseError           = "http.response.error"
	logMsgHTTPResponseEncodeError     = "http.response.encode_error"
	logMsgDatabaseUnavailable         = "api.database_unavailable"
	logMsgClientGone                  = "api.client_gone"
	logMsgAPIResponseError            = "api.response.error"
	logMsgAPIResponseEncodeError      = "api.response.encode_error"
	logMsgHTTPServerError             = "http.server.error"
	logMsgHTTPServerTLSHandshake      = "http.server.tls_handshake_error"
	logMsgHTTPServerServeFailure      = "http.server.serve_failure"
	logMsgAuthStudentLogin            = "auth.student.login" //#nosec:G101
	logMsgAuthAdminLogin              = "auth.admin.login"   //#nosec:G101
	logMsgAuthNonceMismatch           = "auth.nonce_mismatch"
	logMsgAuthLogout                  = "auth.logout"               //#nosec:G101
	logMsgAuthExternalError           = "auth.oidc.external_error"  //#nosec:G101
	logMsgAuthWrongDomain             = "auth.reject.wrong_domain"  //#nosec:G101
	logMsgAuthNotAdmin                = "auth.reject.not_admin"     //#nosec:G101
	logMsgAuthNotStudent              = "auth.reject.not_student"   //#nosec:G101
	logMsgAuthSessionLookupError      = "auth.session.lookup_error" //#nosec:G101
	logMsgReadyzUnavailable           = "readyz.unavailable"
	logMsgShutdownSignal              = "shutdown.signal"
	logMsgShutdownTimeout             = "shutdown.timeout"
	logMsgShutdownComplete            = "shutdown.complete"
	logMsgAdminCategoriesCreate       = "admin.categories.create"
	logMsgAdminCategoriesRename       = "admin.categories.rename"
	logMsgAdminCategoriesDelete       = "admin.categories.delete"
	logMsgAdminPeriodsCreate          = "admin.periods.create"
	logMsgAdminPeriodsRename          = "admin.periods.rename"
	logMsgAdminPeriodsOrder           = "admin.periods.order"
	logMsgAdminPeriodsDelete          = "admin.periods.delete"
	logMsgAdminCoursesCreate          = "admin.courses.create"
	logMsgAdminCoursesUpdate          = "admin.courses.update"
	logMsgAdminCoursesDelete          = "admin.courses.delete"
	logMsgAdminCoursesImport          = "admin.courses.import"
	logMsgAdminGradesCreate           = "admin.grades.create"
	logMsgAdminGradesOrder            = "admin.grades.order"
	logMsgAdminGradesDelete           = "admin.grades.delete"
	logMsgAdminGradesUpdate           = "admin.grades.update"
	logMsgAdminGradesWindow           = "admin.grades.window"
	logMsgAdminGradesOpenNow          = "admin.grades.open_now"
	logMsgAdminGradesCloseNow         = "admin.grades.close_now"
	logMsgAdminGradesCloseAll         = "admin.grades.close_all"
	logMsgAdminGradesBudget           = "admin.grades.budget"
	logMsgAdminGradesRequirements     = "admin.grades.requirements"
	logMsgAdminStudentsUpsert         = "admin.students.upsert"
	logMsgAdminStudentsDelete         = "admin.students.delete"
	logMsgAdminStudentsImport         = "admin.students.import"
	logMsgAdminStudentsExport         = "admin.students.export"
	logMsgAdminCoursesExport          = "admin.courses.export"
	logMsgAdminEnrollmentsPlace       = "admin.enrollments.place"
	logMsgAdminEnrollmentsRemove      = "admin.enrollments.remove"
	logMsgAdminEnrollmentsImport      = "admin.enrollments.import"
	logMsgAdminEnrollmentsExport      = "admin.enrollments.export"
	logMsgAdminDataClear              = "admin.data.clear"
	logMsgWindowTimerArmed            = "window.timer.armed"
	logMsgWindowTimerFired            = "window.timer.fired"
	logMsgWindowTimerError            = "window.timer.error"
	logMsgStudentEnroll               = "student.api.enrollments.enroll"
	logMsgStudentSwap                 = "student.api.enrollments.swap"
	logMsgStudentDrop                 = "student.api.enrollments.drop"
	logMsgAdminEventsUpgradeError     = "admin.api.events.upgrade_error"
	logMsgAdminEventsHelloError       = "admin.api.events.hello_write_error"
	logMsgAdminEventsEstablished      = "admin.api.events.websocket_established"
	logMsgStudentEventsUpgradeError   = "student.api.events.upgrade_error"
	logMsgStudentEventsHelloError     = "student.api.events.hello_write_error"
	logMsgStudentEventsEstablished    = "student.api.events.websocket_established"
	logMsgWebsocketClientRegistered   = "websocket.client.registered"
	logMsgWebsocketClientUnregistered = "websocket.client.unregistered"
	logMsgWebsocketClientEvicted      = "websocket.client.evicted"
	logMsgWebsocketCountsRefreshError = "websocket.counts.refresh_error"
	logMsgWebsocketPingFailed         = "websocket.ping_failed"
	logMsgWebsocketWriteError         = "websocket.write.error"
	logMsgWebsocketClientGone         = "websocket.client_gone"
	logMsgWebsocketReadError          = "websocket.read.error"
	// The service refused to start. One name for every startup
	// failure, because an operator alerts on the name and there was
	// nothing stable to alert on before; the stage attribute says
	// which step gave up.
	logMsgStartupFailed = "startup.failed"

	// Our clock is ahead of the database's: it still calls future a
	// boundary we have already fired for.
	logMsgWindowTimerSkew = "window.timer.skew"

	logMsgStartupConfigLoad     = "startup.config.load"     //#nosec:G101
	logMsgStartupDBConnect      = "startup.db.connect"      //#nosec:G101
	logMsgStartupJWKSFetch      = "startup.jwks.fetch"      //#nosec:G101
	logMsgStartupWebsocketSetup = "startup.websocket.setup" //#nosec:G101
	logMsgStartupRoutesRegister = "startup.routes.register" //#nosec:G101
	logMsgStartupListenerStart  = "startup.listener.start"  //#nosec:G101
	logMsgStartupServing        = "startup.server.serve"    //#nosec:G101
)
