# 터널

[English](tunnels.md).

상태: 구현됨. 이 저장소의 시험에서 공개 터널 서비스로 실행하지는 않았고, unit
시험이 대역 엔진으로 명령을 검사한다.

`bay tunnel <domain>`은 프로젝트의 제공 도메인 하나를 공개 주소로 게시하고 그
주소를 출력한다. 게시는 그 명령이 끝나거나 프로젝트의 컨테이너가 제거되면
끝난다.

## 무엇을 위한 것인가

제공되는 도메인은 이 머신과 승인된 peer에서 답한다. 바깥의 읽는이는 닿을 방법이
없다. 머신이 network에 게시하는 port는 peer link 하나뿐이고(architecture 규칙
7) peer link는 이 머신이 승인한 머신 사이의 상호 TLS로 말한다. 터널은 실행 중인
service에 명령 하나가 도는 동안의 공개 주소를 주고, port는 열지 않는다. 터널
client가 터널 서비스의 edge로 가는 나가는 연결 하나를 맺어 유지할 뿐이다.

```
읽는이 → 터널 서비스의 edge       공개 이름과 그 인증서를 가진다
           ▲
           │ 나가는 연결 하나, client가 유지한다
         <project>-tunnel 컨테이너  이 머신에서, 라우팅된 service의 엔진과
           │                       network 위에서 실행된다
           ▼
         <container>.bay.test:<port>  proxy의 route가 이름하는 것과 같은 이름과
                                      port. runtime이 해석한다
```

## 명령

`bay tunnel <domain>`은 `up`처럼 프로젝트 Compose 파일의 디렉터리에서 실행된다.
도메인은 proxy가 이 프로젝트의 실행 중인 컨테이너에 대해 제공하는 것이어야 한다.
route가 없는 도메인, 컨테이너가 멈춘 도메인, 다른 프로젝트가 쥐었거나 충돌로
잃은 도메인은 모두 그 도메인을 이름하며 거부된다. 게시가 실행 중인 프로젝트는
그것이 끝나기 전까지 두 번째 게시를 거부한다.

게시는 컨테이너 하나다. 이름은 `<project>-tunnel`이고, 라우팅된 컨테이너의
엔진에서 라우팅된 service의 network 위에 만들어지며, `bay.role=service`,
`bay.group`, `bay.service=tunnel` label과 빈 `bay.domain` label을 단다. 프로젝트의
내부 service이므로 route를 갖지 않고, 프로젝트의 컨테이너 목록에 service
`tunnel`로 나타난다.

컨테이너는 터널 client `cloudflare/cloudflared:2026.11.1`을 실행한다. 인자는
라우팅된 service 자신의 주소인 `--url <scheme>://<container>.bay.test:<port>`를
route의 scheme로 이름한다. 기기의 authority를 컨테이너에 읽기 전용으로 붙이고
`--origin-ca-pool`로 가리키며, 뒤쪽 이름을 `--origin-server-name`으로 주어, 자기
TLS를 서비스하는 service를 이 머신의 authority로 검증하는 인증서와 함께 닿게
한다. 이미지는 local 저장소에 없을 때 가져오며, service pull과 같은 이유로 Apple
`container`에서 `--platform`을 함께 준다. `-no-pull`은 가져오기를 거부한다.

명령은 client가 자기 출력에서 주소를 이름하면 공개 주소를 한 번 출력한다. 그
전까지는 기다린다고 밝힌다. 그 이후의 client 출력은 출력하지 않는다. 주소가
이름해지기 전에 client가 끝나면 명령은 그것을 모은 출력과 함께 보고하고
컨테이너를 제거한다. 명령을 끝내는 signal은 컨테이너를 중지하고 제거하므로
중단된 게시는 아무것도 남기지 않는다.

다른 무엇이 컨테이너를 제거할 때도 게시는 같게 끝난다. 프로젝트의 `down`과
`stop`은 이것을 프로젝트의 service 컨테이너 하나로 다루고, 다음 `up`은 Compose
파일이 선언하지 않은 service의 컨테이너로 제거한다.

## 읽는이가 닿는 곳

터널은 service 컨테이너 자신에게, 선언된 port로 전달한다 — proxy의 route가
이름하는 것과 같은 목적지다. proxy가 이 경로에 없으므로 읽는이의 응답은 proxy의
header를 하나도 싣지 않고, 공개 이름은 이 머신의 도메인이 아니라 터널 서비스의
이름이다. 읽는이가 확인하는 인증서는 터널 서비스의 것이다. client와 service
사이에서는, service가 자기 TLS를 서비스할 때 이 머신의 authority가 뒤쪽 이름을
덮는다.

기밀한 경로는 바뀌지 않는다. 머신 사이의 이름은 peer link로 가고, 터널은 자기
명령이 도는 동안 정확히 도메인 하나만 게시한다.
