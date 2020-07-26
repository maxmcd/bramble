"""
I love space at the top of a file
"""


# # these are scripts, how are they interleaved?
# derivation(run=["echo hi", bramble])


def bramble():

    for x in range(10):

        def foo():
            y = x
            print(y)

        derivation(x, foo)


foo(10)
bramble()
