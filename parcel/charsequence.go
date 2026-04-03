package parcel

import "fmt"

// WritePlainCharSequence writes a plain (non-Spanned) CharSequence in the wire
// format used by TextUtils.writeToParcel. The format is:
//
//	int32 kind=1  (plain String)
//	string8 text  (or null string8 if text is nil)
//
// Use nil to write a null CharSequence.
func WritePlainCharSequence(
	p *Parcel,
	text *string,
) {
	p.WriteInt32(1) // kind = plain String
	if text != nil {
		p.WriteString(*text)
	} else {
		p.WriteNullString()
	}
}

// ReadPlainCharSequence reads a CharSequence written by TextUtils.writeToParcel
// and returns the text content. Spanned text is read as plain text (spans are
// skipped). Returns nil for null CharSequences.
func ReadPlainCharSequence(
	p *Parcel,
) (*string, error) {
	kind, err := p.ReadInt32()
	if err != nil {
		return nil, err
	}

	text, err := p.ReadNullableString()
	if err != nil {
		return nil, err
	}

	// For Spanned text (kind != 1), skip the span data.
	if kind != 1 {
		for {
			spanType, err := p.ReadInt32()
			if err != nil {
				return nil, err
			}
			if spanType == 0 {
				break
			}
			if err := skipParcelableSpan(p, spanType); err != nil {
				return nil, fmt.Errorf("span type %d: %w", spanType, err)
			}
		}
	}

	return text, nil
}

// SkipCharSequence reads and discards a CharSequence written by
// TextUtils.writeToParcel. The wire format is:
//
//	int32 kind  (1 = plain String, 2+ = Spanned)
//	string16 text
//	if kind != 1: repeated {int32 spanType, span data, int32[3] where} until spanType==0
func SkipCharSequence(p *Parcel) error {
	kind, err := p.ReadInt32()
	if err != nil {
		return err
	}
	if _, err := p.ReadString16(); err != nil {
		return err
	}
	if kind == 1 {
		return nil
	}
	// Spanned text: loop reading spans until sentinel 0.
	for {
		spanType, err := p.ReadInt32()
		if err != nil {
			return err
		}
		if spanType == 0 {
			break
		}
		if err := skipParcelableSpan(p, spanType); err != nil {
			return fmt.Errorf("span type %d: %w", spanType, err)
		}
	}
	return nil
}

func skipParcelableSpan(p *Parcel, spanType int32) error {
	switch spanType {
	case 1: // ALIGNMENT_SPAN
		if _, err := p.ReadString16(); err != nil {
			return err
		}
	case 2, 3, 4, 7, 9, 12: // single int/float spans
		p.SetPosition(p.Position() + 4)
	case 5, 6, 14, 15, 21: // no-data spans
		// nothing
	case 8: // BULLET_SPAN — gapWidth + wantColor + color
		p.SetPosition(p.Position() + 12)
	case 10: // LEADING_MARGIN_SPAN — first + rest
		p.SetPosition(p.Position() + 8)
	case 11, 13: // URL_SPAN / TYPEFACE_SPAN — string
		if _, err := p.ReadString16(); err != nil {
			return err
		}
	case 16: // ABSOLUTE_SIZE_SPAN — size + dip
		p.SetPosition(p.Position() + 8)
	case 17: // TEXT_APPEARANCE_SPAN
		if _, err := p.ReadString16(); err != nil {
			return err
		}
		p.SetPosition(p.Position() + 20)
		if _, err := p.ReadString16(); err != nil {
			return err
		}
		locFlag, err := p.ReadInt32()
		if err != nil {
			return err
		}
		if locFlag != 0 {
			if _, err := p.ReadString16(); err != nil {
				return err
			}
		}
		p.SetPosition(p.Position() + 16)
		if _, err := p.ReadInt32(); err != nil {
			return err
		}
		if _, err := p.ReadInt32(); err != nil {
			return err
		}
		if _, err := p.ReadString16(); err != nil {
			return err
		}
	case 18: // ANNOTATION — key + value
		if _, err := p.ReadString16(); err != nil {
			return err
		}
		if _, err := p.ReadString16(); err != nil {
			return err
		}
	case 22: // LOCALE_SPAN
		if _, err := p.ReadString16(); err != nil {
			return err
		}
	case 24: // LINE_HEIGHT_SPAN
		p.SetPosition(p.Position() + 4)
	case 25: // LINE_BREAK_CONFIG_SPAN
		p.SetPosition(p.Position() + 8)
	default:
		return fmt.Errorf("unknown span type %d", spanType)
	}
	// where() triple: start, end, flags
	p.SetPosition(p.Position() + 12)
	return nil
}
