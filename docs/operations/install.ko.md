# 설치

머신당 1회 수행하는 설정이다. 이후 모든 작업은 관리자 권한 없이 실행된다.

## 빌드

```sh
make
```

`bin/bay`, `bin/bayd`, `bin/baybar.app`이 생성된다.

## 머신 설정

앱을 열고 **Finish setup**을 선택하거나 다음을 실행한다.

```sh
bin/bay install
```

세 단계가 적용된다.

| 단계 | 관리자 권한 |
| --- | --- |
| 위임할 도메인마다 `/etc/resolver/<도메인>` 작성 | 필요 |
| 인증 기관을 사용자 신뢰 설정에 추가 | 불필요 |
| DNS 서버를 launchd 사용자 에이전트로 등록 | 불필요 |

명령줄은 실행된 터미널에서 `sudo`로 암호를 묻는다. 앱은 시스템 인증 창을 띄우고
번들 안의 `bay` 바이너리로 권한 단계를 실행한다.

## 확인

```sh
bin/bay doctor
```

무엇이 빠졌는지 보고하고 아무것도 바꾸지 않는다. 설정이 끝난 머신은
`nothing to do`를 출력한다.

## 바이너리를 PATH에 두기

```sh
ln -s "$PWD/bin/bay" /usr/local/bin/
ln -s "$PWD/bin/bayd" /usr/local/bin/
```

`bay install`은 `bay` 옆에 있는 `bayd`의 경로로
launchd 에이전트를 등록하므로 두 파일을 같은 위치에 둔다.

## 제거

```sh
bay uninstall
security remove-trusted-cert -d ~/.bay/ca.crt
rm -rf ~/.bay
```

`uninstall`은 resolver 항목과 launchd 에이전트를 제거한다. 인증 기관은 두 번째
명령을 실행할 때까지 신뢰 설정에 남는다. 이 도구가 생성하는 모든 경로는
`docs/spec/machine-state.md`에 있다.
