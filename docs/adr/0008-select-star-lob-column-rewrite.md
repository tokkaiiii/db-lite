# `SELECT *` 단일 테이블 쿼리는 LOB류 컬럼을 SQL 단에서 미리 잘라서 요청한다

**Status**: accepted

ADR 0007의 `truncateCell`은 응답을 브라우저로 보내기 직전, 즉 드라이버가 컬럼 값을 이미 전부 메모리에 올린 뒤에야 자른다. MSSQL의 `nvarchar(max)`에 XML 전체가 들어있는 컬럼처럼 실제 값이 큰 경우, DB 서버 → Go 서버 구간의 네트워크 전송/파싱 비용은 그대로 남아 있어 여전히 느리다. DBeaver 같은 JDBC 기반 도구는 드라이버가 `getSubString(1,N)` 등으로 DB에 "앞부분만 달라"고 요청하거나 LOB을 지연 로딩해서 이 병목 자체가 없다.

같은 효과를 내려면 쿼리를 실행하기 전에 SELECT 절 자체를 고쳐서 DB 단에서부터 잘라 받아야 하는데, 이는 "실행 전에" 판단해야 하므로 ADR 0007이 의도적으로 피했던 **컬럼 타입 조회**가 불가피하다. 이 트레이드오프를 받아들이되, 범위는 최대한 좁혔다: SQL 파서 없이 안전하게 감지 가능한 `SELECT * FROM 단일테이블 [WHERE ...] [ORDER BY ...]` 형태만 재작성 대상으로 삼고, JOIN/서브쿼리/명시적 컬럼 나열/CTE/UNION 등 조금이라도 복잡하면 조용히 건너뛰어 기존 post-fetch truncation에만 의존한다(사용자에게 "최적화 안 됨" 표시도 하지 않는다 — 결과는 항상 정확하고, 이건 있으면 좋은 최적화일 뿐 계약이 아니다).

재작성 대상 컬럼은 길이 제한이 없는 타입(MSSQL `nvarchar(max)`/`varchar(max)`/`varbinary(max)`/`xml`/`text`/`ntext`, MySQL `text`/`blob`류, Postgres `text`/`xml`/`bytea`, Oracle `clob`/`nclob`/`blob`/`long` 등)으로 한정한다 — `varchar(4000)`처럼 길이 제한이 있는 타입은 최악의 경우도 예측 가능해서 post-fetch truncation만으로 충분하다. 이 목록은 실행마다(쿼리가 가리키는 테이블 하나만 콕 집어) `information_schema`류 조회로 얻는다 — 캐시하지 않는다, 이 서버는 커넥션 풀도 세션도 없는 완전 무상태 구조이기 때문(ADR 0004). 대상 컬럼은 `SUBSTRING(col, 1, 2000)`류(MSSQL `xml`, Postgres `xml`처럼 문자열 함수를 바로 못 받는 타입은 각각 `CAST(col AS nvarchar(max))`/`col::text`로 캐스트 선행, Oracle CLOB/BLOB은 `DBMS_LOB.SUBSTR`, 레거시 `LONG`은 `SUBSTR`)으로 감싸 원래 컬럼명으로 alias한다. 이 단계의 자르는 길이(문자 수 기준)는 최종 바이트 상한과 정확히 맞출 필요 없다 — 어차피 `truncateCell`(ADR 0007)이 응답 직전에 다시 한번 정확한 바이트 상한을 보정하는 2차 방어선으로 남아있다.

**검증**: Docker로 MSSQL 2022/MySQL 8.0/Postgres 16/Oracle 23-free를 각각 띄워 `nvarchar(max)`+`xml`+`varbinary(max)`(MSSQL), `longtext`+`longblob`(MySQL), `text`+`xml`+`bytea`(Postgres), `clob`+`blob`(Oracle) 컬럼에 큰 값을 넣고 `SELECT *`/별칭/`WHERE` 조합으로 실행 — 4개 DB 모두 큰 값이 정확히 2000 단위로 잘려 오고, 작은 값은 그대로 오며, 명시적 컬럼 나열처럼 재작성 대상이 아닌 쿼리는 기존 동작(post-fetch truncation만 적용)으로 정확히 폴백함을 확인했다. 이 과정에서 실제 제약 하나를 발견: Oracle은 `USER_TAB_COLUMNS.TABLE_NAME` 비교가 대소문자 구분이라, `FROM docs`(소문자)처럼 쓰면 실제 저장된 `DOCS`와 안 맞아 조회가 비어 재작성이 조용히 스킵된다(`FROM DOCS`로 쓰면 정상 동작). 결과는 항상 정확하므로 이는 버그가 아니라 알려진 최적화 커버리지 한계로 남겨둔다.
