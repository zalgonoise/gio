# gio

*A generic I/O library for Go*

_________


## Overview

Remember the library we all know and love? `"io"`? This library extends the usefulness of the interfaces and functions exposed by the standard library (for byte slices) with generics. The core functionality of a reader (something that reads) should be common amongst any type, provided that the implementation can handle the calls (to read, to write, or whatever action).

This way, despite not having the same fluidity as the standard library's implementation in some levels (there is no `WriteString` function or `StringWriter` interface), it promises to allow the same API to be transposed to other defined types.

Other than this, all functionality from the standard library is present in this generic I/O library.

### Why generics?

Generics are great for when there is a solid algorithm that serves for many types, and can be abstracted enough to work without major workarounds; and this approach to a I/O library is very idiomatic and so simple (the Go way). Of course, the standard library's implementation has some other packages in mind that work together with `io`, such as `strings` and `bytes`. The approach with generics will limit the potential that shines in the original implementation, one way or the other (simply with the fact that if you need to write something to a writer, you need to implement it; there is no basic `WriteString` function for this).

But all in all, it was a great exercise to practice using generics. Maybe I will just use this library once or twice, maybe it will be actually useful for some. I am just in it for the ride. :)


## Disclaimer

This library will mirror all logic from Go's (standard) `io` library; and change the `[]byte` implementation with a generic `T any` and `[]T` implementation. There are no changes in the actual logic in the library.
________________
