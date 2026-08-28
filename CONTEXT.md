# DB 조회/실행 도구 (DBeaver-lite)

사내망에서 여러 종류의 DB(MSSQL/MySQL/PostgreSQL/Oracle)를 조회하고 쿼리를 실행할 수 있게 해주는 웹 기반 도구. Go 서버가 DB에 접근 권한이 있는 위치에서 상시 구동되고, 사용자는 브라우저(추후 Electron)로 접속해 사용한다.

## Language

**User**:
이 앱에 자체 ID/PW로 로그인하는 계정. 실제 DB 계정과는 별개이며, User 자체는 아무 DB에도 접근 권한을 갖지 않는다 — Permission을 통해서만 접근 가능해진다.
_Avoid_: 계정, 로그인 사용자

**Admin**:
Connection을 등록/삭제하고, User에게 Permission을 부여/회수할 수 있는 User의 상위 역할. 일반 User는 이 작업을 할 수 없다.
_Avoid_: 관리자, 운영자

**Connection**:
하나의 DB 서버 인스턴스(호스트+포트+DB 종류+공유 계정)를 가리키는 등록 단위. 권한 부여의 최소 단위이며, 그 서버 안의 개별 Catalog나 테이블 단위로는 세분화하지 않는다.
_Avoid_: DB, 연결정보, 데이터소스

**Catalog**:
Connection 하나가 가리키는 DB 서버 인스턴스 안에 있는, 사용자가 화면에서 고르는 개별 데이터베이스(MySQL/Postgres/MSSQL의 `database`). Permission은 Connection 단위로만 존재하므로 Catalog 선택은 Permission과 무관한 순수 화면 상태이며, 어떤 Catalog를 고르든 그 User의 Connection Permission이 그대로 적용된다. Oracle은 Connection 등록 시 입력하는 서비스명/SID가 이미 접속 대상을 고정하므로 Catalog 개념 자체가 없다.
_Avoid_: 데이터베이스, 스키마, DB

**Permission**:
User와 Connection 사이에 존재하는, 세 등급(`없음`/`읽기`/`쓰기`) 중 하나로 정의되는 접근 수준. `없음`은 해당 Connection이 그 User에게 아예 보이지 않음을, `읽기`는 SELECT만, `쓰기`는 SELECT를 포함한 모든 SQL 실행 가능을 의미한다.
_Avoid_: 권한, 역할, 접근등급

**Write Query**:
SQL 문의 첫 키워드가 조회성(`SELECT`/`SHOW`/`EXPLAIN` 등)이 아닌 모든 구문(INSERT/UPDATE/DELETE/DDL/저장 프로시저 호출 등). Permission이 `쓰기`인 User만 실행할 수 있다.
_Avoid_: 쓰기 쿼리, 변경 쿼리, DML

**Audit Log Entry**:
Write Query가 실행 시도된 사실을 기록한 영속 로그 한 건. 실제로 성공한 실행뿐 아니라, Permission이 `읽기`뿐인 User가 Write Query를 시도했다가 거부된 경우도 포함한다. 조회(SELECT) 실행은 기록하지 않는다.
_Avoid_: 로그, 실행이력, 감사기록

**Row Truncation**:
조회 결과가 `DefaultRowLimit`(1000행)을 넘겨서, 그 이후 행을 아예 내려보내지 않고 잘라낸 상태. `Result.Truncated` 필드로 나타낸다.
_Avoid_: 결과 잘림, 잘린 결과

**Cell Truncation**:
개별 셀 값(주로 BLOB/XML처럼 큰 컬럼)이 2,000바이트를 넘겨서, 값 자체를 잘라 문자열 끝에 `...(잘림, 원본 N바이트)` 마커를 붙인 상태. Row Truncation과 달리 행 자체는 내려가고, 그 행의 특정 셀 값만 축약된다. ([ADR 0007](docs/adr/0007-cell-value-truncation.md))
_Avoid_: 값 잘림, 텍스트 잘림

**Cell Value Download**:
PK가 있는 단일 테이블 `SELECT *` 결과에서, 그 행의 PK 값으로 해당 컬럼만 다시 조회해 잘리지 않은 원본 값을 파일로 내려받는 기능. Cell Truncation으로 그리드에 표시된 축약 값과 달리, 항상 DB의 실제 원본 바이트를 그대로 준다. PK가 없거나 JOIN/서브쿼리처럼 행을 유일하게 식별할 수 없는 결과에는 제공되지 않는다. ([ADR 0009](docs/adr/0009-cell-value-download.md))
_Avoid_: 전체 값 보기, 원본 다운로드
