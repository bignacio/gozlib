#ifndef TEST_ONLY_UTILS_H
#define TEST_ONLY_UTILS_H

#include <zlib.h>
#include <stdlib.h>
#include <assert.h>

#define PRINT_TEST_NAME printf("-- %s\n", __ASSERT_FUNCTION)

typedef void (*InitBufferFn)(char *, uInt);

void init_input_buffer_rand(char *buf, uInt len) {
    for (uInt i = 0; i < len; i++) {
        buf[i] = (char)(rand()%128);
    }
}

void init_input_buffer_high_entropy(char *buf, uInt len) {
    for (uInt i = 0; i < len; i++) {
        buf[i] = (char)(rand()%255);
    }
}


#define ASSERT_MSG(cond, message) assert(cond && message)
#define FAIL_TEST(message) assert(message && false)

#endif
