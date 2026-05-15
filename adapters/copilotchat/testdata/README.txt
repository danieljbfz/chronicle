Test fixtures for the Copilot adapter.

Each .jsonl file is a small, real-shape Copilot chat session captured
for unit testing. The records mirror what VS Code Copilot Chat
actually writes -- same kind values, same field names, same nesting
-- with all identifying content replaced.

Files under v3/ describe the format observed in VS Code with Copilot
Chat schema version 3. When VS Code ships a breaking change to its
on-disk format, a new subdirectory v4/ (or whatever the new schema
version number is) appears alongside, with one fixture per distinct
change.

The fixtures by purpose:

  v3/empty_session.jsonl
      A session that was created and then never used. The snapshot
      has zero requests. IsAbandoned() returns true on the parsed
      Conversation. This fixture covers the abandoned-session
      detection that drives the cleanup feature.

  v3/small_session.jsonl
      A snapshot followed by a few mutation events that build up
      one user request and one assistant reply. Smallest fixture
      that exercises the full event-log replay along with text and
      a tool invocation.

  synthetic_future.jsonl
      The CANARY. Contains a snapshot, one normal event, and one
      event with an unknown kind value. The replay test asserts
      that the unknown kind lands in result.UnknownKinds and that
      the rest of the replay still produces a usable snapshot.
      This is the single fixture that proves the resilience contract
      holds for the Copilot adapter: when VS Code ships a new event
      kind, chronicle keeps reading instead of crashing.
