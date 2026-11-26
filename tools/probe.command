#!/bin/zsh
set -eu
cd -- "${0:A:h}/.."
export JAVA_HOME="${JAVA_HOME:-/opt/homebrew/opt/openjdk@17/libexec/openjdk.jdk/Contents/Home}"
export ANDROID_HOME="${ANDROID_HOME:-$HOME/Library/Android/sdk}"
probe_temp=$(mktemp -d)
trap 'rm -rf "$probe_temp"' EXIT
mkdir -p "$probe_temp/classes" "$probe_temp/dex"
"$JAVA_HOME/bin/javac" -cp "$ANDROID_HOME/platforms/android-35/android.jar" -d "$probe_temp/classes" tools/CapabilityProbe.java
"$ANDROID_HOME/build-tools/35.0.0/d8" --lib "$ANDROID_HOME/platforms/android-35/android.jar" --output "$probe_temp/dex" "$probe_temp/classes/CapabilityProbe.class"
"$ANDROID_HOME/platform-tools/adb" push "$probe_temp/dex/classes.dex" /data/local/tmp/sheldon-capability.dex
"$ANDROID_HOME/platform-tools/adb" shell 'CLASSPATH=/data/local/tmp/sheldon-capability.dex app_process -Xhidden-api-policy:disabled /system/bin CapabilityProbe'
"$ANDROID_HOME/platform-tools/adb" shell rm /data/local/tmp/sheldon-capability.dex
