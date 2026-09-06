#!/bin/sh
# Build a .deb or .rpm from an already-built binary.
#
# Runs inside the distribution's own container with that distribution's own tools, because
# a package built by a third-party tool is a package whose behaviour on install is a guess.
#
#   build.sh deb|rpm ARCH VERSION BINARY OUTDIR
set -eu

format=$1
arch=$2
version=$3
binary=$4
outdir=$5

root=$(mktemp -d)
trap 'rm -rf "$root"' EXIT

install -D -m 0755 "$binary" "$root/usr/bin/datum"
install -D -m 0644 packaging/agent.yaml "$root/etc/datum/agent.yaml"
mkdir -p "$root/lib/systemd/system"
python3 packaging/extract-unit.py > "$root/lib/systemd/system/datum.service"
chmod 0644 "$root/lib/systemd/system/datum.service"

# The state directory holds the accepted revision, the pass lock and pass reports. The
# agent refuses to run if it is not 0700, so the package creates it correctly rather than
# leaving the first pass to fail.
install -d -m 0700 "$root/var/lib/datum"

mkdir -p "$outdir"

# Both packages depend on git and on ssh-keygen. The agent shells out to git, and git needs
# ssh-keygen to check a signature made with an ssh key, which is the default.
case $format in
deb)
  debarch=$arch
  [ "$debarch" = "aarch64" ] && debarch=arm64
  [ "$debarch" = "x86_64" ] && debarch=amd64

  mkdir -p "$root/DEBIAN"
  cat > "$root/DEBIAN/control" <<EOF
Package: datum
Version: $version
Section: admin
Priority: optional
Architecture: $debarch
Maintainer: dsgnr <mail@getdatum.sh>
Depends: git, openssh-client
Homepage: https://getdatum.sh/
Description: Reconciles a Linux host against desired state held in Git
 Datum reads a repository, works out what differs on the host, applies the
 difference and verifies the result. A machine acquires its identity at
 enrolment, so installing this package configures nothing on its own.
EOF

  # conffiles keeps a local edit to agent.yaml through an upgrade. Without it dpkg would
  # replace a file holding the host's identity.
  printf '/etc/datum/agent.yaml\n' > "$root/DEBIAN/conffiles"

  # The unit is installed and deliberately not enabled. A freshly installed machine has
  # no identity, so an enabled agent would fail every pass until somebody enrolled it.
  cat > "$root/DEBIAN/postinst" <<'EOF'
#!/bin/sh
set -e
if [ "$1" = configure ]; then
    systemctl daemon-reload >/dev/null 2>&1 || true
fi
EOF
  chmod 0755 "$root/DEBIAN/postinst"

  cat > "$root/DEBIAN/prerm" <<'EOF'
#!/bin/sh
set -e
if [ "$1" = remove ]; then
    systemctl stop datum.service >/dev/null 2>&1 || true
fi
EOF
  chmod 0755 "$root/DEBIAN/prerm"

  dpkg-deb --root-owner-group --build "$root" "$outdir/datum_${version}_${debarch}.deb" >/dev/null
  ;;

rpm)
  rpmarch=$arch
  [ "$rpmarch" = "arm64" ] && rpmarch=aarch64
  [ "$rpmarch" = "amd64" ] && rpmarch=x86_64

  spec=$root/../datum.spec
  cat > "$spec" <<EOF
Name:           datum
Version:        $version
Release:        1
Summary:        Reconciles a Linux host against desired state held in Git
License:        MIT
URL:            https://getdatum.sh/
BuildArch:      $rpmarch
Requires:       git
Requires:       openssh-clients

# A Go binary is already stripped of what rpm wants to strip, carries no debug link and
# has no changelog to date the build from, so all three of rpm's defaults are turned off.
%global _build_id_links none
%global __strip /bin/true
%global source_date_epoch_from_changelog 0

%description
Datum reads a repository, works out what differs on the host, applies the
difference and verifies the result. A machine acquires its identity at
enrolment, so installing this package configures nothing on its own.

%install
mkdir -p %{buildroot}
cp -a $root/usr %{buildroot}/
cp -a $root/etc %{buildroot}/
cp -a $root/lib %{buildroot}/
mkdir -p -m 0700 %{buildroot}/var/lib/datum

%files
%attr(0755, root, root) /usr/bin/datum
%config(noreplace) %attr(0644, root, root) /etc/datum/agent.yaml
%attr(0644, root, root) /lib/systemd/system/datum.service
%dir %attr(0755, root, root) /etc/datum
%dir %attr(0700, root, root) /var/lib/datum

%post
systemctl daemon-reload >/dev/null 2>&1 || true

%preun
if [ \$1 -eq 0 ]; then
    systemctl stop datum.service >/dev/null 2>&1 || true
fi
EOF

  rpmbuild --quiet --define "_topdir $root/rpmbuild" --define "_rpmdir $outdir" \
           --define "_rpmfilename datum-$version-1.$rpmarch.rpm" \
           -bb "$spec" >/dev/null
  ;;

*)
  echo "unknown format $format" >&2
  exit 1
  ;;
esac
