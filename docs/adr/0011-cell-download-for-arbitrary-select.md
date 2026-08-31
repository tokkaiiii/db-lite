# 셀 다운로드를 JOIN 포함 임의의 SELECT로 확장하고, 이를 위해 정규식 대신 SQL 파서를 도입한다

**Status**: accepted (4방언 JOIN까지 구현 완료 — 아래 "구현 현황" 참고, 서브쿼리/CTE/UNION은 아직 후속 작업)

ADR 0009는 셀 다운로드(그리드에 표시된 잘린 값이 아니라 DB의 원본 바이트를 재조회)를 "PK가 있는 단일 테이블 `SELECT *`"로 좁혔다. 이유는 "이 셀 값이 정확히 어느 행에서 왔는지" 알아낼 방법이 좁은 정규식 파싱(ADR 0008) 밖에서는 없었기 때문이다. 사용자가 JOIN을 포함한 임의의 SELECT 결과에서도 셀 다운로드를 쓰고 싶어 해서, 이번에 그 범위를 넓힌다. **지원 범위**는 JOIN뿐 아니라 서브쿼리, CTE, UNION, 명시적 컬럼 목록까지 포함한 사실상 임의의 SELECT로 정한다.

이 범위를 지원하려면 "출력 컬럼이 어느 테이블의 어느 컬럼에서 왔는지"를 알아내야 하는데, JOIN·서브쿼리·UNION이 섞인 SQL에서 이를 정적으로 알아내려면 정규식으로는 불가능하고 **진짜 SQL 파서가 필요하다**. ADR 0008/0009는 "틀린 리라이트가 결과를 조용히 바꿀 수 있다"는 이유로 파서 대신 좁은 정규식을 택했다 — 이번 결정은 그 원칙을 뒤집는 것이다. 이 트레이드오프를 받아들이는 대신, 아래 안전장치(재조회값-그리드값 교차검증)로 "파서가 틀렸을 때 조용히 엉뚱한 값을 내려주는" 최악의 실패 모드를 막는다.

## 설계

- **DB별 파서**: DB Lite가 지원하는 MySQL/Postgres/MSSQL/Oracle 네 방언 모두 정식 지원한다. 방언마다 문법이 달라(TOP/LIMIT/ROWNUM, 인용 규칙 등) 각각 별도 파서 라이브러리가 필요하고(예: MySQL은 vitess/tidb 계열, Postgres는 pg_query_go 계열, MSSQL은 tsqlparser 계열, Oracle은 bytebase/omni 또는 plsql-parser 계열), 서로 다른 AST를 "alias → 테이블", "출력 컬럼 → 원본 테이블.컬럼"이라는 공통 매핑으로 정규화하는 계층을 하나 둔다.
- **PK 자동 주입**: 파서가 어떤 출력 컬럼의 원본 테이블을 알아냈어도, 그 테이블의 PK 컬럼이 SELECT 결과에 없으면 재조회로 행을 특정할 수 없다. ADR 0008이 LOB 컬럼을 서버에서 선제 처리하듯, 서버가 내부적으로 각 원본 테이블의 PK 컬럼을 `__pk_<alias>_<col>` 같은 이름으로 리라이트한 쿼리에 추가해 함께 조회한다. 이 컬럼들은 그리드에 노출하지 않고 재조회에만 쓴다.
- **서브쿼리/CTE**: 출력 컬럼이 파생 테이블(서브쿼리/CTE)에서 왔으면 그 안을 재귀적으로 따라가 실제 원본 테이블까지 추적한다. 도중에 집계나 컬럼 변형(예: `SUM(...)`, `a || b`)이 끼면 그 지점에서 원본 추적을 포기한다.
- **UNION**: 버치마다 같은 출력 컬럼 위치가 다른 테이블에서 올 수 있으므로, 행 단위로 "이 행이 어느 버치에서 왔는지"를 태깅해 버치별로 출처를 개별 추적한다.
- **추적 실패 시 컬럼 단위 fail-closed**: 파서가 특정 컬럼의 원본을 확신할 수 없으면(집계/연산식 결과, 윈도우 함수, 상관 서브쿼리, 파서가 모르는 문법 등) 쿼리 전체가 아니라 **그 컬럼만** 다운로드를 비활성화한다. 뷰(View)에서 온 컬럼은 별도 처리 없이, 기존 `PrimaryKeyColumns` 조회가 PK를 못 찾으면 자연히 비활성화된다.
- **오탐지 안전장치**: 다운로드 요청이 오면 파서가 추적한 table+PK로 재조회한 값을, 그리드에 이미 표시된 값(2000바이트로 잘려 있을 수 있으므로 같은 길이만큼 잘라서)과 문자열로 비교한다. 다르면 파서가 출처를 잘못 추적했거나 재조회 사이에 데이터가 바뀐 것이므로 500 에러로 다운로드를 거부하고 로그를 남긴다 — "비활성화되어야 할 다운로드가 조용히 엉뚱한 값을 내려주는" 실패를 막는 마지막 방어선이다.
- **API 스키마 변경**: `Result.Table string` / `Result.PrimaryKey []string`(결과 전체에 값 하나)로는 컬럼별·행별로 다른 출처를 표현할 수 없다. `ColumnOrigins []*ColumnOrigin`(컬럼마다 Table+PrimaryKey, 추적 불가 시 nil)과 행별 버치 태그로 교체한다. 프론트엔드의 `canDownloadCell`/`downloadCell`도 결과 전체 단위가 아니라 셀 단위로 다운로드 가능 여부를 판단하도록 다시 작성한다.

**범위 밖**: 디비버 스타일 인앱 값 뷰어는 ADR 0009와 마찬가지로 범위 밖이다. 파서 라이브러리 선정과 방언별 정규화 계층의 구체적 구현은 이 ADR이 아니라 별도 구현 계획에서 다룬다.

## 구현 현황

실제 구현은 "일반 JOIN(파생 테이블 없는 `t1 [AS] a1 JOIN t2 [AS] a2 ON ...`)"부터 좁혀 시작했고, 방언은 MySQL → Postgres → MSSQL → Oracle 순서로 하나씩 붙여 각각 Docker 컨테이너로 실제 검증했다. 서브쿼리/CTE 재귀 추적과 UNION 버치 태깅은 아직 후속 작업(wayfinder 이슈)으로 남아 있다.

**방언별 파서**:
- MySQL — `github.com/xwb1989/sqlparser` (순수 Go, vitess 초기 포크)
- Postgres — `github.com/wasilibs/go-pgquery`(파싱/deparse) + `github.com/pganalyze/pg_query_go/v6`(AST 타입·노드 생성 헬퍼). go-pgquery는 실제 Postgres 파서를 WASM으로 컴파일해 cgo 없이 크로스컴파일 가능하게 만든 pg_query_go의 드롭인 대체품 — 리턴 타입이 pg_query_go와 동일한 protobuf라 헬퍼 함수를 그대로 재사용한다.
- MSSQL — `github.com/ha1tch/tsqlparser` (순수 Go, T-SQL 전용 recursive-descent 파서, `.String()`으로 재직렬화 가능)
- Oracle — `github.com/bytebase/plsql-parser`(ANTLR 생성 PL/SQL 문법) + `github.com/antlr4-go/antlr/v4`. 처음엔 `github.com/bytebase/omni`(더 높은 수준의 래퍼)를 시도했으나, 이걸 추가하자 `go mod tidy`가 MySQL/Postgres/MSSQL/Oracle 드라이버 자체 버전까지 줄줄이 끌어올렸다 — 이 기능과 무관한 위험한 부작용이라 되돌리고 더 가벼운 `plsql-parser`(ANTLR 문법만, 드라이버 의존성 없음)로 바꿨다.

**Oracle 파서의 리라이트 방식이 다른 세 방언과 다르다**: xwb1989/sqlparser·go-pgquery·tsqlparser는 모두 AST를 고치고 다시 SQL 문자열로 직렬화하는 `String()`/`Deparse()` 함수를 제공하지만, ANTLR로 생성된 plsql-parser에는 그런 역직렬화 기능이 없다. 대신 파싱된 `selected_list`의 마지막 토큰이 원본 문자열의 몇 번째 문자에서 끝나는지 찾아, 그 위치 바로 뒤에 숨은 PK 컬럼 텍스트를 직접 이어붙인다 — ADR 0008의 정규식 리라이트가 원래 하던 것과 같은 발상의 텍스트 스플라이싱을, 이번엔 정규식 대신 실제 파스 트리가 찾아준 위치를 기준으로 한 것. 이때 ANTLR Go 런타임의 토큰 위치는 **바이트가 아니라 룬(rune) 단위**라, 스플라이싱도 `[]rune`으로 변환해서 처리해야 한다(이 프로젝트는 한국어 사용자 대상이라 테이블/컬럼명에 비ASCII 문자가 실제로 나올 수 있음).

**Oracle은 대소문자 폴딩이 이미 알려진 제약**: ADR 0008 당시 이미 발견된 사실(`internal/dbconn/lob.go`의 주석 참고)로, Oracle은 따옴표 없는 식별자를 항상 대문자로 저장하고 `USER_TAB_COLUMNS`/PK 조회는 대소문자를 구분한다. 즉 쿼리를 `FROM users`(소문자)로 쓰면 실제 카탈로그의 `USERS`와 매치되지 않아 PK를 못 찾고 다운로드가 자연히 비활성화된다 — 새 버그가 아니라 기존 ADR 0008 rewrite와 동일한 fail-open 동작이 그대로 이어진 것이다. 이번 JOIN 경로도 같은 이유로 대문자로 쓴 쿼리(`FROM USERS U JOIN ORDERS O ...`)에서만 다운로드가 활성화되는 것을 Docker Oracle로 확인했다.

**GROUP BY/DISTINCT 가드**: 첫 MySQL 구현을 Docker MySQL로 검증하는 과정에서 예상 못 한 제약을 하나 발견했다 — PK 자동 주입(위 "PK 누락 시" 항목)이 **`GROUP BY`가 있는 쿼리를 깨뜨릴 수 있다**. MySQL의 `only_full_group_by` 모드가 GROUP BY에 없는 컬럼이 SELECT 목록에 추가되는 것을 거부해, 원래 잘 실행되던 쿼리가 에러로 바뀌었다. `DISTINCT`도 비슷하게 위험하다 — 숨겨진 PK 컬럼이 추가되면 어떤 행이 "distinct"로 취급되는지가 조용히 바뀔 수 있다(에러가 아니라 잘못된 결과). 그래서 4개 방언 모두, 파서 경로는 `GROUP BY`/`HAVING`/`DISTINCT`가 있는 문장은 통계를 손대지 않고 통째로 건너뛴다(그 쿼리에 대해서는 다운로드 기능 전체를 비활성화).
