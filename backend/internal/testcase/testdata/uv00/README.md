# UV-00 testcase fixtures

`versioning-cases.json` is synthetic. It contains no production data,
credentials, or asserted reviewer provenance. It captures two versioning edge
cases before UV-02 changes the implementation:

1. two distinct negative scenarios currently collide on one logical key;
2. step/test-data-only edits currently keep the same generation key.
