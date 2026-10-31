#define TINY_MAX_URL 2048
#define TINY_ERR_TOO_LONG 3

struct tiny_url {
  char *buf;
  char *host;
};

int tiny_parse_url(const char *text, struct tiny_url *out);
int tiny_split_url(struct tiny_url *u);
void tiny_free_url(struct tiny_url *u);
