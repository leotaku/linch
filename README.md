# Linch

[![Github Actions CI](https://github.com/leotaku/linch/workflows/check/badge.svg)](https://github.com/leotaku/linch/actions)
[![Go Report Card](https://goreportcard.com/badge/github.com/leotaku/linch)](https://goreportcard.com/report/github.com/leotaku/linch)
[![RFC 7231](https://img.shields.io/badge/RFC-7231-66F)](https://tools.ietf.org/html/rfc7231#section-6)

Linch is a simplistic non-recursive link validator.

## Usage

``` shell
echo README.md | linch
find ../notes | linch
```

Linch check on filepaths that are passed on standard input. This way it can easily be composed with other command line tools.
Passed directories are ignored silently and not checked.

``` shell
fd | linch --sed-mode | parallel -j1
```

Linch offers a special *sed-mode* in which it outputs [sed](https://en.wikipedia.org/wiki/Sed) commands to fix permanent redirects in your local files.
Many sed commands may edit the same file so ensure that they are always run sequentially.

## License

[MIT](./LICENSE) © Leo Gaskin 2020-2021
