# TODO

## Candidates

These are design opportunities for consideration, not committed work.

- Provide a reusable capture session that owns the display, buffers, producer, frame leases, and cleanup, so examples do not need to assemble the lifecycle themselves.
- Establish consistent ownership conventions for Binder registrations, parcel descriptors, graphics buffers, fences, cancellation, partial failures, and retryable cleanup.
- Expose target protocol capabilities per connection, including remote connections, so callers do not need release-specific flags or infer target behavior from the host.
- Separate graphics allocation and pixel access from vendor-specific layouts behind reusable buffer capabilities; evaluate the experimental Android hardware-buffer route before adopting it.
- Preserve AIDL import and type identity throughout generation, with a staged migration and tests for ambiguous names and generated API compatibility.
- Make generated-code verification faster and more reliable: avoid repeated extraction, use an isolated checkout, and detect untracked generated files as well as tracked diffs.
- Add protocol fixtures and consumer-level compatibility tests for wire formats, ownership, null values, and integration contracts such as STF's PID handshake.
