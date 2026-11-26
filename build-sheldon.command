#!/bin/zsh
set -eu
cd -- "${0:A:h}"
export JAVA_HOME="${JAVA_HOME:-/opt/homebrew/opt/openjdk@17/libexec/openjdk.jdk/Contents/Home}"
export ANDROID_HOME="${ANDROID_HOME:-$HOME/Library/Android/sdk}"
export GOTOOLCHAIN=go1.26.3
export PATH="$JAVA_HOME/bin:$HOME/go/bin:$PATH"
exec ./gradlew assembleDebug --no-daemon
