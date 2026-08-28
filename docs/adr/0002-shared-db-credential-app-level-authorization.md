# Connection당 공유 DB 계정 + 앱 레벨 권한 관리

각 User가 자신의 DB 계정으로 직접 접속하는 대신, Connection마다 서버가 보유한 공유 DB 계정 하나로 접속하고, User별 접근/쓰기 가능 여부는 앱(Permission)이 판단한다. User마다 실제 DB 계정을 발급/관리하는 운영 부담을 피하기 위한 선택이다.

트레이드오프: DB 자체의 계정별 권한/감사 기능을 활용하지 못하므로, 접근 통제와 쓰기 이력 추적 책임이 전적으로 이 앱(Permission, Audit Log Entry)에 있다. DB 서버 로그만 보고는 어떤 User가 실행한 쿼리인지 알 수 없다.
