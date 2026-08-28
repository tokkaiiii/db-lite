# CloudBeaver 대신 자체 Go 서버 구현

CloudBeaver가 요구사항(서버가 DB에 접근, 클라이언트는 별도)을 이미 충족하지만, 대상 Windows Server에 Docker와 Java 설치가 불가능해 컨테이너/JVM 기반 배포를 쓸 수 없다. 이 제약 때문에 단일 실행 파일로 배포 가능한 Go로 최소 기능의 서버를 직접 구현하기로 했다. Docker나 Java 설치가 가능해지면 이 결정은 재검토 대상이다.
