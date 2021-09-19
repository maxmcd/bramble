from lxml import etree
import random

tree = etree.parse(open("./bramble.svg"))

# points taken top to bottom along the bush border, we don't want sparkles left
# of the border
border = [
    [121, 16],
    [113, 72],
    [121, 97],
    [131, 113],
    [176, 156],
    [207, 196],
    [220, 210],
]


def closest(lst, k):
    return lst[min(range(len(lst)), key=lambda i: abs(lst[i] - K))]


def left_of_border(x, y):
    i = min(range(len(border)), key=lambda i: abs(border[i][1] - y))
    # print(y, border[i][1])
    return x < border[i][0]


def opacity():
    # generate pairs of random numbers and make sure that the same value is at
    # the beginning and end of the sequence. This ensures there is no
    # "reset-flash" when the cycle restarts
    out = []
    for _ in range(5):
        v = str(
            0.5 if random.random() > 0.2 else 1
        )  # change 1 to >1 to get a mix of hard transitions and fades
        out += [v] + [v]
    # Shift
    return out[1:] + out[:1]


assert left_of_border(420, 231) == False
assert left_of_border(17, 117) == True

for i, node in enumerate(tree.iter()):
    if not (hasattr(node.tag, "endswith") and node.tag.endswith("}circle")):
        continue
    y = float(node.get("cy"))
    x = float(node.get("cx"))
    if left_of_border(x, y):
        continue

    if i % 10 == 0:

        node.append(
            etree.fromstring(
                '<animate attributeName="opacity" values="%s" dur="2s" repeatCount="indefinite" />'
                % ";".join(opacity())
            )
        )


print(etree.tostring(tree).decode("utf-8"))

