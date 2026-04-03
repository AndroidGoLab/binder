package content

import (
	"fmt"

	"github.com/AndroidGoLab/binder/parcel"
)

// MIMETypePlainText is the MIME type for plain text clipboard entries.
const MIMETypePlainText = "text/plain"

// MarshalPlainTextClipData writes a ClipData containing a single plain-text
// item to the parcel. This matches ClipData.newPlainText(label, text) on the
// Java side.
//
// Wire format (matching ClipData.writeToParcel + ClipDescription.writeToParcel):
//
//	ClipDescription:
//	  CharSequence label          (TextUtils.writeToParcel: kind=1, string8)
//	  StringList   mimeTypes      (writeStringList: count, then string16 per entry)
//	  PersistableBundle extras    (null = int32(-1))
//	  int64        timestamp      (0)
//	  bool         isStyledText   (false)
//	  int32        classStatus    (0)
//	  Bundle       confidences    (null = int32(-1))
//	int32 iconFlag (0 = no icon)
//	int32 N        (1 item)
//	Item:
//	  CharSequence text           (TextUtils.writeToParcel: kind=1, string8)
//	  string8      htmlText       (null = int32(-1))
//	  TypedObject  intent         (null = int32(0))
//	  TypedObject  intentSender   (null = int32(0))
//	  TypedObject  uri            (null = int32(0))
//	  TypedObject  activityInfo   (null = int32(0))
//	  TypedObject  textLinks      (null = int32(0))
func MarshalPlainTextClipData(
	p *parcel.Parcel,
	label string,
	text string,
) {
	// ClipDescription
	parcel.WritePlainCharSequence(p, &label)                    // mLabel
	p.WriteStringList([]string{MIMETypePlainText})              // mMimeTypes
	p.WriteInt32(-1)                                            // mExtras (null PersistableBundle)
	p.WriteInt64(0)                                             // mTimeStamp
	p.WriteBool(false)                                          // mIsStyledText
	p.WriteInt32(0)                                             // mClassificationStatus
	p.WriteInt32(-1)                                            // confidences bundle (null)

	// Icon
	p.WriteInt32(0) // no icon

	// Items
	p.WriteInt32(1) // N = 1

	// Item[0]
	parcel.WritePlainCharSequence(p, &text) // mText
	p.WriteNullString()                     // mHtmlText (null string8)
	p.WriteInt32(0)                         // mIntent (null typed object)
	p.WriteInt32(0)                         // mIntentSender (null typed object)
	p.WriteInt32(0)                         // mUri (null typed object)
	p.WriteInt32(0)                         // mActivityInfo (null typed object)
	p.WriteInt32(0)                         // mTextLinks (null typed object)
}

// ClipDataText holds the text content extracted from a ClipData parcel.
type ClipDataText struct {
	Label     string
	MIMETypes []string
	Items     []string
}

// UnmarshalClipDataText reads a ClipData from the parcel and extracts plain
// text content. Non-text fields (intents, URIs, etc.) are skipped.
//
// This reads the same wire format written by MarshalPlainTextClipData and by
// Android's ClipData.writeToParcel.
func UnmarshalClipDataText(
	p *parcel.Parcel,
) (ClipDataText, error) {
	var result ClipDataText

	// ClipDescription
	label, err := parcel.ReadPlainCharSequence(p)
	if err != nil {
		return result, fmt.Errorf("reading label: %w", err)
	}
	if label != nil {
		result.Label = *label
	}

	mimeTypes, err := p.ReadStringList()
	if err != nil {
		return result, fmt.Errorf("reading mimeTypes: %w", err)
	}
	result.MIMETypes = mimeTypes

	// Skip PersistableBundle (extras): null = -1, otherwise length-prefixed.
	if err := skipBundle(p); err != nil {
		return result, fmt.Errorf("skipping extras: %w", err)
	}

	// timestamp, isStyledText, classificationStatus
	if _, err := p.ReadInt64(); err != nil {
		return result, fmt.Errorf("reading timestamp: %w", err)
	}
	if _, err := p.ReadBool(); err != nil {
		return result, fmt.Errorf("reading isStyledText: %w", err)
	}
	if _, err := p.ReadInt32(); err != nil {
		return result, fmt.Errorf("reading classificationStatus: %w", err)
	}

	// Skip confidences bundle
	if err := skipBundle(p); err != nil {
		return result, fmt.Errorf("skipping confidences: %w", err)
	}

	// Icon
	iconFlag, err := p.ReadInt32()
	if err != nil {
		return result, fmt.Errorf("reading icon flag: %w", err)
	}
	if iconFlag != 0 {
		// Bitmap follows; we cannot skip it without knowing the wire size.
		return result, fmt.Errorf("non-null icon (Bitmap): unsupported")
	}

	// Items
	n, err := p.ReadInt32()
	if err != nil {
		return result, fmt.Errorf("reading item count: %w", err)
	}

	for i := int32(0); i < n; i++ {
		text, err := parcel.ReadPlainCharSequence(p)
		if err != nil {
			return result, fmt.Errorf("item[%d] text: %w", i, err)
		}

		// htmlText (string8, nullable)
		if _, err := p.ReadNullableString(); err != nil {
			return result, fmt.Errorf("item[%d] htmlText: %w", i, err)
		}

		// intent (typed object)
		if err := skipTypedObject(p); err != nil {
			return result, fmt.Errorf("item[%d] intent: %w", i, err)
		}

		// intentSender (typed object)
		if err := skipTypedObject(p); err != nil {
			return result, fmt.Errorf("item[%d] intentSender: %w", i, err)
		}

		// uri (typed object)
		if err := skipTypedObject(p); err != nil {
			return result, fmt.Errorf("item[%d] uri: %w", i, err)
		}

		// activityInfo (typed object)
		if err := skipTypedObject(p); err != nil {
			return result, fmt.Errorf("item[%d] activityInfo: %w", i, err)
		}

		// textLinks (typed object)
		if err := skipTypedObject(p); err != nil {
			return result, fmt.Errorf("item[%d] textLinks: %w", i, err)
		}

		if text != nil {
			result.Items = append(result.Items, *text)
		}
	}

	return result, nil
}

// skipBundle skips a Bundle or PersistableBundle in the parcel.
// Null bundles are represented as int32(-1). Non-null bundles have an int32
// byte length followed by that many bytes of data.
func skipBundle(
	p *parcel.Parcel,
) error {
	length, err := p.ReadInt32()
	if err != nil {
		return err
	}
	if length > 0 {
		p.SetPosition(p.Position() + int(length))
	}
	return nil
}

// skipTypedObject skips a typed object written by Java's writeTypedObject.
// Null objects have flag=0. Non-null objects have flag!=0 followed by
// variable-length data that we cannot skip without knowing the type.
func skipTypedObject(
	p *parcel.Parcel,
) error {
	flag, err := p.ReadInt32()
	if err != nil {
		return err
	}
	if flag != 0 {
		return fmt.Errorf("non-null typed object: cannot skip without known wire format")
	}
	return nil
}
