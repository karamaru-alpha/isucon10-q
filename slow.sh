#!/bin/sh

sudo pt-query-digest $1 --limit=5 --report-format=query_report --filter="$event->{arg} =~ m/^select/i"'
