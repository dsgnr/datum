# A container that boots systemd as PID 1, for the systemd provider's integration
# tests. systemctl is present in plenty of images where nothing booted it and every
# call fails on the bus, so testing against that would prove nothing.
#
# cron is here because it has no ExecReload and nginx because it has one. The
# difference between a reload and a restart is what two of the tests check.
FROM golang:1.25

RUN apt-get update -qq && \
    DEBIAN_FRONTEND=noninteractive apt-get install -y -qq --no-install-recommends \
        systemd systemd-sysv cron nginx-light && \
    rm -rf /var/lib/apt/lists/*

# systemd expects this signal to shut down cleanly.
STOPSIGNAL SIGRTMIN+3

CMD ["/sbin/init"]
