#!/bin/bash

TESTS=releaselist go test -v -run TestScanner_releaselist &> unused.txt
