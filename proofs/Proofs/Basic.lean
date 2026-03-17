-- Formal specification of the Android Binder parcel wire format.
-- Models the Go implementation in parcel/*.go.

namespace Binder.Parcel

/-! ## Alignment

The Go code uses `(n + 3) &^ 3` for 4-byte alignment.
This is equivalent to `((n + 3) / 4) * 4` on natural numbers.
-/

def align4 (n : Nat) : Nat := ((n + 3) / 4) * 4

theorem align4_ge (n : Nat) : n ≤ align4 n := by
  unfold align4; omega

theorem align4_mod4 (n : Nat) : align4 n % 4 = 0 := by
  unfold align4; omega

theorem align4_minimal (n m : Nat) (hm : n ≤ m) (hmod : m % 4 = 0) :
    align4 n ≤ m := by
  unfold align4; omega

theorem align4_idempotent (n : Nat) : align4 (align4 n) = align4 n := by
  unfold align4; omega

theorem align4_zero : align4 0 = 0 := by decide

theorem align4_of_mul4 (k : Nat) : align4 (4 * k) = 4 * k := by
  unfold align4; omega

theorem align4_bound (n : Nat) : align4 n < n + 4 := by
  unfold align4; omega

theorem align4_padding_range (n : Nat) : align4 n - n < 4 := by
  unfold align4; omega

theorem align4_mono (a b : Nat) (h : a ≤ b) : align4 a ≤ align4 b := by
  unfold align4; omega

theorem align4_add_aligned (a b : Nat) (ha : a % 4 = 0) (hb : b % 4 = 0) :
    align4 (a + b) = a + b := by
  unfold align4; omega

/-! ## Parcel model -/

structure ParcelState where
  len : Nat
  pos : Nat
  deriving Repr, DecidableEq

def ParcelState.empty : ParcelState := ⟨0, 0⟩

/-! ## Grow: extend buffer by aligned amount -/

def ParcelState.grow (p : ParcelState) (n : Nat) : ParcelState × Nat :=
  (⟨p.len + align4 n, p.pos⟩, p.len)

theorem ParcelState.grow_len (p : ParcelState) (n : Nat) :
    (p.grow n).1.len = p.len + align4 n := by
  simp [ParcelState.grow]

theorem ParcelState.grow_start (p : ParcelState) (n : Nat) :
    (p.grow n).2 = p.len := by
  simp [ParcelState.grow]

theorem ParcelState.grow_preserves_pos (p : ParcelState) (n : Nat) :
    (p.grow n).1.pos = p.pos := by
  simp [ParcelState.grow]

theorem ParcelState.grow_aligned (p : ParcelState) (n : Nat) (h : p.len % 4 = 0) :
    (p.grow n).1.len % 4 = 0 := by
  simp [ParcelState.grow]
  have := align4_mod4 n
  omega

/-! ## Read: consume n bytes with aligned advance -/

def ParcelState.read (p : ParcelState) (n : Nat) : Option ParcelState :=
  let aligned := align4 n
  if p.pos + aligned ≤ p.len then
    some ⟨p.len, p.pos + aligned⟩
  else
    none

theorem ParcelState.read_advances (p : ParcelState) (n : Nat)
    (h : p.pos + align4 n ≤ p.len) :
    p.read n = some ⟨p.len, p.pos + align4 n⟩ := by
  simp [ParcelState.read, h]

theorem ParcelState.read_preserves_len (p : ParcelState) (n : Nat) (p' : ParcelState)
    (h : p.read n = some p') : p'.len = p.len := by
  simp [ParcelState.read] at h
  obtain ⟨_, rfl⟩ := h
  rfl

/-! ## Write-then-read round-trip (structural) -/

theorem write_read_int32 :
    let p := ParcelState.empty
    let (p', _) := p.grow 4
    p'.read 4 = some ⟨4, 4⟩ := by
  simp [ParcelState.empty, ParcelState.grow, ParcelState.read, align4]

theorem write_read_int64 :
    let p := ParcelState.empty
    let (p', _) := p.grow 8
    p'.read 8 = some ⟨8, 8⟩ := by
  simp [ParcelState.empty, ParcelState.grow, ParcelState.read, align4]

theorem write_write_read_read_int32 :
    let p0 := ParcelState.empty
    let (p1, _) := p0.grow 4
    let (p2, _) := p1.grow 4
    match p2.read 4 with
    | some p3 => p3.read 4 = some ⟨8, 8⟩
    | none => False := by
  simp [ParcelState.empty, ParcelState.grow, ParcelState.read, align4]

theorem write_read_roundtrip (n : Nat) :
    let p := ParcelState.empty
    let (p', _) := p.grow n
    p'.read n = some ⟨align4 n, align4 n⟩ := by
  simp [ParcelState.empty, ParcelState.grow, ParcelState.read]

/-! ## Parcelable header/footer protocol -/

theorem parcelable_size_correct (headerPos payloadSize : Nat) :
    let currentLen := headerPos + 4 + payloadSize
    currentLen - headerPos = 4 + payloadSize := by omega

theorem parcelable_endPos_correct (startPos size : Nat) :
    let endPos := startPos + size
    endPos - startPos = size := by omega

theorem parcelable_forward_compat (startPos knownFields extraFields : Nat)
    (hSize : size = 4 + knownFields + extraFields) :
    let endPos := startPos + size
    endPos = startPos + 4 + knownFields + extraFields := by omega

/-! ## Binder object null detection -/

def BINDER_TYPE_BINDER : Nat := 0x73622a85
def BINDER_TYPE_HANDLE : Nat := 0x73682a85

inductive BinderResult where
  | null : BinderResult
  | handle : Nat → BinderResult
  | error : BinderResult
  deriving DecidableEq, Repr

def classifyBinder (objType : Nat) (handle : Nat) : BinderResult :=
  if objType = 0 then .null
  else if objType ≠ BINDER_TYPE_HANDLE ∧ objType ≠ BINDER_TYPE_BINDER then .error
  else if objType = BINDER_TYPE_BINDER ∧ handle = 0 then .null
  else .handle handle

-- WriteNullStrongBinder → null
theorem writeNull_classifies_null :
    classifyBinder BINDER_TYPE_BINDER 0 = .null := by
  native_decide

-- WriteStrongBinder with nonzero handle → handle
theorem writeHandle_classifies_handle (_h : handle ≠ 0) :
    classifyBinder BINDER_TYPE_HANDLE handle = .handle handle := by
  unfold classifyBinder BINDER_TYPE_HANDLE BINDER_TYPE_BINDER
  simp

-- BINDER_TYPE_HANDLE with handle=0 is NOT treated as null
-- (only BINDER_TYPE_BINDER with binder=0 is null)
theorem writeHandle_zero_not_null :
    classifyBinder BINDER_TYPE_HANDLE 0 = .handle 0 := by
  native_decide

-- WriteLocalBinder with nonzero ptr → handle
theorem writeLocal_classifies_handle (h : ptr ≠ 0) :
    classifyBinder BINDER_TYPE_BINDER ptr = .handle ptr := by
  unfold classifyBinder BINDER_TYPE_BINDER
  simp
  omega

-- All-zero → null
theorem allZero_classifies_null :
    classifyBinder 0 0 = .null := by native_decide

-- Invalid type → error
theorem invalidType_classifies_error
    (h1 : t ≠ 0) (h2 : t ≠ BINDER_TYPE_HANDLE) (h3 : t ≠ BINDER_TYPE_BINDER) :
    classifyBinder t handle = .error := by
  unfold classifyBinder
  simp [h1]
  intro h4
  exact absurd (h4 h2) h3

/-! ### ReadStrongBinder (non-nullable) — after bug fix

The original Go code accepted null binders in ReadStrongBinder (non-nullable).
The Lean proof showed that classifyBinder BINDER_TYPE_HANDLE handle = .handle handle
WITHOUT needing handle ≠ 0 — revealing that ReadStrongBinder needed an
explicit null check for BINDER_TYPE_BINDER with binder=0.

The fixed ReadStrongBinder now rejects null, modeled by classifyNonNullBinder below.
-/

-- Non-nullable read: rejects null, returns handle or error.
def classifyNonNullBinder (objType : Nat) (handle : Nat) : BinderResult :=
  if objType = 0 then .error  -- all-zero is rejected by type check
  else if objType ≠ BINDER_TYPE_HANDLE ∧ objType ≠ BINDER_TYPE_BINDER then .error
  else if objType = BINDER_TYPE_BINDER ∧ handle = 0 then .error  -- null → error (the fix!)
  else .handle handle

-- Non-nullable read never returns .null
theorem nonNull_never_null (objType handle : Nat) :
    classifyNonNullBinder objType handle ≠ .null := by
  unfold classifyNonNullBinder
  split
  · exact BinderResult.noConfusion
  · split
    · exact BinderResult.noConfusion
    · split
      · exact BinderResult.noConfusion
      · exact BinderResult.noConfusion

-- For valid non-null binders, both nullable and non-nullable agree
theorem nullable_nonnull_agree_on_valid (objType handle : Nat)
    (hv : classifyBinder objType handle = .handle handle) :
    classifyNonNullBinder objType handle = .handle handle := by
  unfold classifyBinder at hv
  unfold classifyNonNullBinder
  split
  · split at hv <;> simp_all
  · split
    · split at hv <;> simp_all
    · split
      · split at hv <;> simp_all
      · rfl

-- Completeness: every input is classified
theorem classifyBinder_total (objType handle : Nat) :
    classifyBinder objType handle = .null ∨
    (∃ h, classifyBinder objType handle = .handle h) ∨
    classifyBinder objType handle = .error := by
  unfold classifyBinder
  split
  · left; rfl
  · split
    · right; right; rfl
    · split
      · left; rfl
      · right; left; exact ⟨handle, rfl⟩

/-! ## flat_binder_object layout -/

def flatBinderObjectSize : Nat := 24
def binderObjectWireSize : Nat := flatBinderObjectSize + 4

theorem flatBinderObject_aligned : align4 flatBinderObjectSize = flatBinderObjectSize := by
  native_decide

theorem binderObjectWire_aligned : align4 binderObjectWireSize = binderObjectWireSize := by
  native_decide

/-! ## Stability levels -/

def stabilityUndeclared : Nat := 0
def stabilitySystem : Nat := 12

theorem stabilitySystem_is_0b001100 : stabilitySystem = 0b001100 := by decide

/-! ## Interface token -/

def interfaceTokenPrefixSize : Nat := 12

theorem interfaceToken_prefix_aligned :
    align4 interfaceTokenPrefixSize = interfaceTokenPrefixSize := by
  native_decide

/-! ## UTF-8 string wire size -/

def stringUTF8WireSize (byteLen : Nat) : Nat :=
  4 + align4 (byteLen + 1)

theorem stringUTF8WireSize_aligned (n : Nat) :
    stringUTF8WireSize n % 4 = 0 := by
  unfold stringUTF8WireSize; have := align4_mod4 (n + 1); omega

theorem stringUTF8_empty_size : stringUTF8WireSize 0 = 8 := by native_decide

/-! ## UTF-16 string wire size -/

def string16WireSize (charCount : Nat) : Nat :=
  4 + align4 ((charCount + 1) * 2)

theorem string16WireSize_aligned (n : Nat) :
    string16WireSize n % 4 = 0 := by
  unfold string16WireSize; have := align4_mod4 ((n + 1) * 2); omega

theorem string16_empty_size : string16WireSize 0 = 8 := by native_decide

-- Odd charCount: (odd+1) is even, (even)*2 is div by 4, no padding
theorem string16_odd_charCount_no_pad (k : Nat) :
    align4 ((2 * k + 1 + 1) * 2) = (2 * k + 1 + 1) * 2 := by
  unfold align4; omega

-- Even nonzero charCount: (even+1) is odd, (odd)*2 ≡ 2 mod 4, needs 2 bytes padding
theorem string16_even_charCount_has_pad (k : Nat) :
    align4 ((2 * k + 1) * 2) = (2 * k + 1) * 2 + 2 := by
  unfold align4; omega

/-! ## Byte array wire format -/

def byteArrayWireSize (len : Nat) : Nat := 4 + align4 len

theorem byteArrayWireSize_aligned (n : Nat) :
    byteArrayWireSize n % 4 = 0 := by
  unfold byteArrayWireSize; have := align4_mod4 n; omega

theorem byteArray_empty_size : byteArrayWireSize 0 = 4 := by native_decide

def fixedByteArrayWireSize (fixedSize : Nat) : Nat := 4 + align4 fixedSize

theorem fixedByteArray_eq_byteArray (n : Nat) :
    fixedByteArrayWireSize n = byteArrayWireSize n := by
  simp [fixedByteArrayWireSize, byteArrayWireSize]

/-! ## Composition: sequential writes maintain alignment -/

theorem sequential_writes_aligned (len₀ n₁ n₂ : Nat) (h₀ : len₀ % 4 = 0) :
    (len₀ + align4 n₁ + align4 n₂) % 4 = 0 := by
  have h₁ := align4_mod4 n₁
  have h₂ := align4_mod4 n₂
  omega

theorem empty_aligned : ParcelState.empty.len % 4 = 0 := by
  simp [ParcelState.empty]

theorem grow_chain_aligned (p : ParcelState) (h : p.len % 4 = 0) (n : Nat) :
    (p.grow n).1.len % 4 = 0 :=
  ParcelState.grow_aligned p n h

end Binder.Parcel
