cmake_minimum_required(VERSION 3.0)
project(zwrapper VERSION 1.0)

add_compile_options(
    -Wall
    -Wpedantic
    -Werror
    -Wextra
    -Wconversion
    -Wsign-conversion
    -Walloca
    -Wshadow
    -Wfloat-equal
    -Wswitch-enum
    -Wcast-qual
    -Wimplicit-fallthrough
    -Wundef
    -Wfloat-equal
    -Werror=missing-field-initializers
    -g
)

find_package(ZLIB)
add_executable(zwrapper_test_direct gozlib.c test_direct.c)
add_executable(zwrapper_test_stream gozlib.c test_stream.c)

target_link_libraries(zwrapper_test_stream ZLIB::ZLIB)
target_link_libraries(zwrapper_test_direct ZLIB::ZLIB)
