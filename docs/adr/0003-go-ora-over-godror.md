# Oracle 드라이버로 go-ora 채택 (godror 대신)

godror가 더 성숙하지만 CGO 기반이라 서버에 Oracle Instant Client 설치가 필요하다. 이 프로젝트의 시작 동기 자체가 "설치가 번거로운 것을 피하는 것"이므로, 순수 Go 구현이라 별도 클라이언트 설치가 필요 없는 go-ora를 선택했다. LOB 처리 등 일부 고급 기능이 필요해지면 재검토한다.
