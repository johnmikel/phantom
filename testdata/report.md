# Heap overflow in tiny_parse_url

The bug is in `lib/url.c` at line 9, where tiny_parse_url() copies the URL without checking its length.
It is reached through `tiny_decode_url()`, and the `TINY_MAX_URL` limit is ignored.
The comment at lib/url.c:7 even says so.

The vulnerable code:

```c
size_t len = strlen(text);
memcpy(out->buf, text, len + 1);
tiny_unsafe_copy(out->buf, text);
```

See also src/url_parser.c line 40, and line 400 of lib/url.c.

Run it with `tiny --insecure --follow-redirects http://example.com`.

## Proof of concept

```c
#include <stdio.h>
int main(void) {
  char *payload = make_payload(4096);
  return trigger_overflow(payload);
}
```

Compile with `gcc poc.c -o poc` and run it under `valgrind --leak-check=full ./poc`.

Recommended fix: replace memcpy with `tiny_safe_copy()`.
