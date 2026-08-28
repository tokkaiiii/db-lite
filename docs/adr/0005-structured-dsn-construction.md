# DB 접속 문자열(DSN)은 구조화된 빌더로 생성

MSSQL/Postgres/Oracle의 DSN을 `fmt.Sprintf("scheme://%s:%s@%s:%d", user, pass, host, port)` 식으로 문자열을 직접 이어붙여 만들고 있었다. 비밀번호에 `/`, `@`, `#`, `?` 같은 URL 특수문자가 들어가면 그 지점에서 URL의 authority(호스트) 구간이 조기에 끝난 것으로 파싱되어, 사용자명과 비밀번호 앞부분이 host:port로 오인되고 실제 host/port는 통째로 사라지는 방식으로 깨졌다. 실사용자가 MSSQL에서 이 버그로 연결 자체를 못 하는 것을 겪은 뒤 발견했다.

`net/url.URL`에 `User: url.UserPassword(...)`을 채워 `.String()`으로 직렬화하면 특수문자가 자동으로 퍼센트 인코딩되어 이 문제가 근본적으로 사라진다. MSSQL/Postgres는 이 방식으로, Oracle은 go-ora가 제공하는 `BuildUrl` 헬퍼로, MySQL은 문자열 DSN이 아니라 go-sql-driver가 제공하는 `mysql.Config.FormatDSN()`으로 바꿨다. 앞으로 DSN을 만질 때 절대 문자열을 직접 조립하지 말고, 각 드라이버가 제공하는 구조화된 빌더를 우선 찾아 쓴다.
