# Patchup
[![License](https://img.shields.io/github/license/mashape/apistatus.svg)](https://github.com/catamat/patchup/blob/master/LICENSE)
[![Go Report Card](https://goreportcard.com/badge/github.com/catamat/patchup)](https://goreportcard.com/report/github.com/catamat/patchup)
[![Go Reference](https://pkg.go.dev/badge/github.com/catamat/patchup.svg)](https://pkg.go.dev/github.com/catamat/patchup)
[![Version](https://img.shields.io/github/tag/catamat/patchup.svg?color=blue&label=version)](https://github.com/catamat/patchup/releases)

Patchup is a simple command line tool to increment the patch part of your version number so you just have to think about major and minor parts.
It will parse a source file to find and increment a patch var/const, it will also update a timestamp var/const if you need it.
Target variables or constants must be package-level declarations assigned explicit string literals.
Patchup should be called before each build.
The patch target is required; the command exits with an error if it cannot be found.
The timestamp target is optional by default, and you can disable it explicitly with `-tn ""`.
Pass `-pn ""` if you want to update only the timestamp target.

## Installation:
```
go install github.com/catamat/patchup@latest
```

## Options:
```
-fn [string]
	Go source file name (default "version.go")

-pn [string]
	Patch var/const name (default "versionPatch")

-tf [string]
	Timestamp output format (default "0601021504")

-tn [string]
	Timestamp var/const name (default "versionTimestamp")
```

## Usage:
Before:
```
// version.go

package main

const versionMajor = "1"
const versionMinor = "0"
const versionPatch = "0"
const versionTimestamp = ""

const version = versionMajor + "." + versionMinor + "." + versionPatch
```
Command:

```
./patchup -fn version.go
```

After:
```
// version.go

package main

const versionMajor = "1"
const versionMinor = "0"
const versionPatch = "1"
const versionTimestamp = "2010051551"

const version = versionMajor + "." + versionMinor + "." + versionPatch
```
