#!/usr/bin/env python3
"""Pull the systemd unit out of the documentation.

The unit has one copy, in docs/lifecycle/running.md, and the package ships that copy. Two
copies would drift, and the one people read would be the wrong one.
"""
import re
import sys

doc = sys.argv[1] if len(sys.argv) > 1 else 'docs/lifecycle/running.md'
title = sys.argv[2] if len(sys.argv) > 2 else '/etc/systemd/system/datum.service'

text = open(doc).read()
pattern = re.compile(r'```ini title="' + re.escape(title) + r'"\n(.*?)```', re.S)
match = pattern.search(text)
if not match:
    sys.exit(f'{doc} has no ini block titled {title}')
sys.stdout.write(match.group(1))
