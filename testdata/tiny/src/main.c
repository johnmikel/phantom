#include <stdio.h>
#include <string.h>
#include "../lib/url.h"

static int insecure, verbose;

int main(int argc, char **argv)
{
  for(int i = 1; i < argc; i++) {
    if(!strcmp(argv[i], "--insecure"))
      insecure = 1;
    else if(!strcmp(argv[i], "--verbose"))
      verbose = 1;
  }
  return 0;
}
