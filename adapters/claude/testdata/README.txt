Test fixtures for the Claude adapter.

Each .jsonl file is a small, real-shape Claude Code session captured for
unit testing. The records are edited copies of what Claude Code actually
writes — same field names, same nesting, same record types — with all
identifying content replaced.

Files under v1_0/ describe the format observed in Claude Code 2.1.x. When
Claude Code ships a breaking change to its on-disk format, a new
subdirectory v1_1/ (or similar) appears alongside, with one fixture per
distinct change.

The fixtures by purpose:

  v1_0/empty_session.jsonl
      A session that was opened, ran /clear, and never received a real
      prompt. The IsAbandoned() check returns true for this. Used to
      verify the abandoned-session detection that drives
      `chronicle clean abandoned`.

  v1_0/small_session.jsonl
      Three turns: user prompt -> assistant text + tool_use -> user
      tool_result -> assistant text. The smallest fixture that exercises
      the full block taxonomy except thinking and image. The parser test
      asserts that text, tool_use, and tool_result all survive.

  v1_0/thinking_session.jsonl
      One assistant turn with both a thinking block and a text block.
      Used to verify the parser keeps thinking blocks (we display them
      behind a filter toggle, but we do not drop them).

  synthetic_future.jsonl
      The CANARY. Contains a fabricated record type
      ("future-event-from-tomorrow") and a fabricated assistant content
      kind ("galaxy_brain"). The parser test asserts that BOTH survive
      as UnknownBlock entries in the parsed Conversation. This is the
      single fixture that proves the resilience contract holds: when
      Claude Code ships a new record type or content kind, chronicle
      surfaces it instead of crashing.

      If a future change to parse.go ever drops these unknowns, the
      canary test fails immediately and loud — exactly what we want.
