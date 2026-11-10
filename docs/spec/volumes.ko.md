# Named volume

[English](volumes.md) · [생명주기](service-lifecycle.ko.md)

관리·external named volume을 지원한다. 로컬 검증은 바이너리를 설치하거나 기존 애플리케이션 데이터를 이전하지 않는다.

최상위 `volumes`는 선언 키와 선택적 `name`, `external`, `driver: local`, `driver_opts.size`를 받는다. 알 수 없는 driver와 옵션은 거부한다. 관리 이름의 기본값은 `<project>-<key>`다. `size`는 양수 바이트 수이며 1024의 거듭제곱을 쓰는 K·M·G·T·P 접미사를 붙일 수 있다. 크기가 없으면 런타임 기본값을 사용한다. External 선언은 `external: true`와 선택적 `name`만 지원하며 이미 존재해야 한다.

컨테이너 시작 전에 선택한 모든 볼륨을 검사한다. 관리 볼륨은 정확한 프로젝트·선언 키·volume-role·설정 label을 가져야 하며 이름이 같다는 이유로 소유권을 부여하지 않는다. 기존 볼륨은 예상한 local driver와 선언한 크기가 맞아야 한다. 불일치는 오류이며 자동 인수·크기 조정·교체를 하지 않는다. 기존 볼륨 검사를 모두 통과하면 프로젝트 생명주기 잠금 안에서 없는 관리 볼륨을 생성한다. External volume은 존재 여부만 검사하며 label을 바꾸지 않는다. 각 `container volume inspect`는 15초의 자기 read bound 아래에서, 각 `container volume create`는 10분의 자기 action bound 아래에서 실행된다. 한 `up`의 volume 명령은 deadline을 공유하지 않는데, 공유된 deadline은 batch가 그 deadline보다 오래 걸리면 정상인 명령을 끝내기 때문이다. bound가 끝낸 명령은 명령, context error와 경과 시간으로 실패한다.

`down`은 소유 컨테이너를 제거하고 관리·external 볼륨을 모두 보존한다. 반복 `up`은 같은 볼륨과 데이터를 사용한다. DB·queue·cache 추가는 Compose 설정으로 표현하며 애플리케이션별 볼륨 생성 코드가 필요하지 않다. 관리 볼륨 크기 변경은 이 명령 밖에서 운영자가 명시적으로 데이터를 이전해야 한다. 도구는 불일치한 선언을 거부한다.

검증: `make check`와 `go test -race ./internal/stack ./internal/contract`가 선언 검증·소유권·정확한 크기·기본 이름·변경 없는 재사용·external 동작을 검사한다. `BAY_SERVICE_E2E=1 go test -race ./internal/stack -run '^TestManagedVolumeActualRuntime$' -count=1 -timeout=120s`는 고유한 64 MiB 볼륨과 컨테이너 하나를 만든다. 변경 없는 시작과 컨테이너 제거·재생성 뒤 데이터 보존을 확인하고 다른 프로젝트 및 크기 변경을 거부한 뒤 소유한 fixture 자원만 제거한다. 기존 모든 컨테이너를 보존하며 공유 proxy/DNS와 애플리케이션 DB는 변경하지 않는다.
