#!/bin/sh
# Check an installed package, from inside the container that installed it.
#
# Two things are being checked. The layout is the one the documentation promises, and
# installing has configured nothing, since a freshly installed machine has no identity and
# an enabled agent would fail every pass until somebody enrolled it.
set -eu

fail() { echo "FAIL: $*" >&2; exit 1; }

echo "--- the documented layout"
[ -x /usr/bin/datum ] || fail "/usr/bin/datum is missing or not executable"
[ -f /etc/datum/agent.yaml ] || fail "/etc/datum/agent.yaml is missing"
[ -f /lib/systemd/system/datum.service ] || fail "the unit is missing"
[ -d /var/lib/datum ] || fail "/var/lib/datum is missing"

echo "--- nothing beyond the defaults is in /etc/datum"
extra=$(ls -A /etc/datum | grep -v '^agent.yaml$' || true)
[ -z "$extra" ] || fail "/etc/datum also holds $extra"

# The agent refuses to run if its state directory is not 0700, so a package that got this
# wrong would install cleanly and then fail every pass.
echo "--- the directories carry the modes the agent expects"
for pair in /etc/datum:755 /var/lib/datum:700; do
    dir=${pair%:*}
    want=${pair#*:}
    mode=$(stat -c '%a' "$dir")
    [ "$mode" = "$want" ] || fail "$dir is mode $mode, want $want"
done

echo "--- the state directory is empty, because no pass has run"
[ -z "$(ls -A /var/lib/datum)" ] || fail "/var/lib/datum is not empty"

echo "--- the binary runs, so it is built for this machine and needs no libraries"
/usr/bin/datum --help >/dev/null 2>&1 || fail "datum --help failed"

# A package whose binary cannot say which version it is leaves an operator guessing, and the
# version it reports has to be the one the package was built as.
echo "--- the binary reports the version it was packaged as"
reported=$(/usr/bin/datum version | head -1)
[ "$reported" = "datum ${1:-0.1.0~dev}" ] || fail "datum version said '$reported'"

echo "--- git and ssh-keygen are there, because verifying a revision uses both"
command -v git >/dev/null || fail "the package did not pull in git"
command -v ssh-keygen >/dev/null || fail "the package did not pull in ssh-keygen"

echo "--- installing does not enrol, so the shipped config names no host"
if grep -qE '^[[:space:]]*host:' /etc/datum/agent.yaml; then
    fail "the shipped config names a host"
fi

echo "--- and the agent says so rather than guessing one"
if /usr/bin/datum config check >/dev/null 2>&1; then
    fail "config check passed on a machine that has not been enrolled"
fi
/usr/bin/datum config check 2>&1 | grep -q 'host is required' \
    || fail "config check failed for some reason other than the missing host"

echo "--- status still reports, so an unenrolled machine is observable"
/usr/bin/datum status >/dev/null || fail "datum status failed before enrolment"

# Checked as the symlink rather than through systemctl, because these images have no
# systemd in them and enabling a unit is that symlink either way.
echo "--- the unit is installed and not enabled"
for wants in /etc/systemd/system/*.wants/datum.service; do
    if [ -e "$wants" ]; then
        fail "installing enabled the service: $wants"
    fi
done

echo "--- the unit names the installed binary"
grep -q '^ExecStart=/usr/bin/datum' /lib/systemd/system/datum.service \
    || fail "ExecStart does not point at /usr/bin/datum"

echo "ok"
