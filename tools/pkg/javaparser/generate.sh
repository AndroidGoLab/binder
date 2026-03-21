#!/usr/bin/env bash
# Regenerate Go code from ANTLR .g4 grammars, then fix unreachable-code vet warnings.
#
# Requirements: Java 11+ and antlr-4.13.1-complete.jar (downloaded automatically).
set -euo pipefail

cd "$(dirname "$0")"

ANTLR_VERSION=4.13.1
ANTLR_JAR="${ANTLR_JAR:-$HOME/.local/lib/antlr-${ANTLR_VERSION}-complete.jar}"

# Download the ANTLR jar if not present.
if [ ! -f "$ANTLR_JAR" ]; then
    mkdir -p "$(dirname "$ANTLR_JAR")"
    echo "Downloading antlr-${ANTLR_VERSION}-complete.jar..."
    curl -fSL -o "$ANTLR_JAR" "https://www.antlr.org/download/antlr-${ANTLR_VERSION}-complete.jar"
fi

echo "Generating Go code from .g4 grammars..."
java -jar "$ANTLR_JAR" \
    -Dlanguage=Go \
    -package javaparser \
    -no-visitor \
    -listener \
    JavaLexer.g4 JavaParser.g4

# ANTLR 4.13.1 Go target emits unreachable "goto errorExit" after return statements
# as a trick to suppress "label defined and not used" errors. This causes go vet
# failures. Remove these dead lines, and also remove orphaned errorExit: label
# blocks when the label has no other live references.
echo "Post-processing: removing unreachable goto errorExit lines..."
python3 -c "
with open('java_parser.go') as f:
    lines = f.readlines()

DEAD_GOTO = '\tgoto errorExit // Trick to prevent compiler error if the label is not used\n'

# Identify function boundaries (top-level func ... { ... }).
func_ranges = []
func_start = None
for i, line in enumerate(lines):
    if line.startswith('func '):
        func_start = i
    elif line == '}\n' and func_start is not None:
        func_ranges.append((func_start, i))
        func_start = None

lines_to_remove = set()

for start, end in func_ranges:
    dead_indices = [i for i in range(start, end+1) if lines[i] == DEAD_GOTO]
    if not dead_indices:
        continue

    # Count live 'goto errorExit' lines (exact match, excluding dead ones).
    live_gotos = 0
    for i in range(start, end+1):
        if i in dead_indices:
            continue
        if lines[i].strip() == 'goto errorExit':
            live_gotos += 1

    # Always remove dead gotos.
    for idx in dead_indices:
        lines_to_remove.add(idx)

    # If no live gotos remain, also remove the errorExit: label and its
    # if-p.HasError() block (up to but not including p.ExitRule()/return).
    if live_gotos == 0:
        for i in range(start, end+1):
            if lines[i].strip() == 'errorExit:':
                lines_to_remove.add(i)
                j = i + 1
                while j <= end:
                    s = lines[j].strip()
                    if s == 'p.ExitRule()' or s.startswith('return '):
                        break
                    lines_to_remove.add(j)
                    j += 1
                break

with open('java_parser.go', 'w') as f:
    for i, line in enumerate(lines):
        if i not in lines_to_remove:
            f.write(line)

print(f'Removed {len(lines_to_remove)} unreachable lines from java_parser.go')
"

echo "Done. Run 'go vet ./tools/pkg/javaparser/' to verify."
