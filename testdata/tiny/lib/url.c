/* url.c: parsing URLs for tiny */
#include <stdlib.h>
#include <string.h>
#include "url.h"

/*
 * tiny_parse_url splits a URL into its parts.
 */
int tiny_parse_url(const char *text, struct tiny_url *out)
{
  size_t len = strlen(text);
  if(len > TINY_MAX_URL)
    return TINY_ERR_TOO_LONG;
  memcpy(out->buf, text, len + 1);
  return tiny_split_url(out);
}

void tiny_free_url(struct tiny_url *u)
{
  free(u->buf);
}
