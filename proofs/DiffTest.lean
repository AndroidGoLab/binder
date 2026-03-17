-- Differential test oracle: computes expected results for parcel encoding test vectors.
-- Output format: one line per test, "CATEGORY KEY RESULT"
-- The Go test harness runs the same computations and compares.

import Proofs

open Binder.Parcel

-- Helpers for output

def printLine (category key result : String) : IO Unit :=
  IO.println s!"{category} {key} {result}"

-- Alignment test vectors: input → align4(input)
def alignmentTests : IO Unit := do
  let cases := [0, 1, 2, 3, 4, 5, 6, 7, 8, 9, 10, 11, 12,
                15, 16, 17, 23, 24, 25, 31, 32, 33,
                100, 255, 256, 1000, 1023, 1024, 4096, 65535, 65536]
  for n in cases do
    printLine "ALIGN4" (toString n) (toString (align4 n))

-- String UTF-8 wire size: byteLen → total wire size
def stringUTF8Tests : IO Unit := do
  let cases := [0, 1, 2, 3, 4, 5, 6, 7, 8, 10, 15, 16, 31, 32, 100, 255, 256, 1000]
  for n in cases do
    printLine "STRING_UTF8_SIZE" (toString n) (toString (stringUTF8WireSize n))

-- String UTF-16 wire size: charCount → total wire size
def string16Tests : IO Unit := do
  let cases := [0, 1, 2, 3, 4, 5, 6, 7, 8, 10, 15, 16, 31, 32, 100, 255, 256, 1000]
  for n in cases do
    printLine "STRING16_SIZE" (toString n) (toString (string16WireSize n))

-- Byte array wire size: length → total wire size
def byteArrayTests : IO Unit := do
  let cases := [0, 1, 2, 3, 4, 5, 6, 7, 8, 10, 15, 16, 31, 32, 100, 255, 256, 1000]
  for n in cases do
    printLine "BYTEARRAY_SIZE" (toString n) (toString (byteArrayWireSize n))

-- Binder classification: (objType, handle) → result
def binderClassifyTests : IO Unit := do
  let cases : List (Nat × Nat × String) := [
    -- null cases
    (0, 0, "NULL"),
    (0, 42, "NULL"),
    (BINDER_TYPE_BINDER, 0, "NULL"),
    -- handle cases
    (BINDER_TYPE_HANDLE, 0, "HANDLE:0"),
    (BINDER_TYPE_HANDLE, 1, "HANDLE:1"),
    (BINDER_TYPE_HANDLE, 42, "HANDLE:42"),
    (BINDER_TYPE_HANDLE, 0xFFFFFFFF, "HANDLE:4294967295"),
    (BINDER_TYPE_BINDER, 1, "HANDLE:1"),
    (BINDER_TYPE_BINDER, 42, "HANDLE:42"),
    (BINDER_TYPE_BINDER, 0x12345678, "HANDLE:305419896"),
    -- error cases
    (1, 0, "ERROR"),
    (0xDEADBEEF, 0, "ERROR"),
    (0x73622a84, 0, "ERROR"),  -- one less than BINDER_TYPE_BINDER
    (0x73682a86, 0, "ERROR")   -- one more than BINDER_TYPE_HANDLE
  ]
  for (objType, handle, expected) in cases do
    let result := classifyBinder objType handle
    let resultStr := match result with
      | .null => "NULL"
      | .handle h => s!"HANDLE:{h}"
      | .error => "ERROR"
    printLine "BINDER_CLASSIFY" s!"{objType},{handle}" resultStr
    -- Verify against expected (the Lean side is the oracle, but double-check)
    if resultStr != expected then
      IO.eprintln s!"INTERNAL ERROR: expected {expected}, got {resultStr} for ({objType},{handle})"

-- Non-nullable binder classification
def binderNonNullTests : IO Unit := do
  let cases : List (Nat × Nat × String) := [
    (0, 0, "ERROR"),
    (BINDER_TYPE_BINDER, 0, "ERROR"),  -- null → error in non-nullable
    (BINDER_TYPE_HANDLE, 0, "HANDLE:0"),
    (BINDER_TYPE_HANDLE, 42, "HANDLE:42"),
    (BINDER_TYPE_BINDER, 1, "HANDLE:1"),
    (1, 0, "ERROR"),
    (0xDEADBEEF, 0, "ERROR")
  ]
  for (objType, handle, _expected) in cases do
    let result := classifyNonNullBinder objType handle
    let resultStr := match result with
      | .null => "NULL"
      | .handle h => s!"HANDLE:{h}"
      | .error => "ERROR"
    printLine "BINDER_NONNULL" s!"{objType},{handle}" resultStr

-- Parcelable size tests: (headerPos, payloadSize) → totalSize
def parcelableTests : IO Unit := do
  let cases := [(0, 0), (0, 4), (0, 100), (12, 0), (12, 8), (100, 200), (0, 1000)]
  for (headerPos, payloadSize) in cases do
    let totalSize := 4 + payloadSize
    let endPos := headerPos + totalSize
    printLine "PARCELABLE_SIZE" s!"{headerPos},{payloadSize}" s!"{totalSize},{endPos}"

-- Grow/read structural tests: starting from empty, grow n, then read n
def growReadTests : IO Unit := do
  let cases := [0, 1, 2, 3, 4, 5, 6, 7, 8, 12, 16, 24, 28, 32, 100]
  for n in cases do
    let p := ParcelState.empty
    let (p', start) := p.grow n
    let readResult := p'.read n
    match readResult with
    | some p'' =>
      printLine "GROW_READ" (toString n) s!"{p'.len},{start},{p''.pos}"
    | none =>
      printLine "GROW_READ" (toString n) "FAIL"

def main : IO Unit := do
  alignmentTests
  stringUTF8Tests
  string16Tests
  byteArrayTests
  binderClassifyTests
  binderNonNullTests
  parcelableTests
  growReadTests
