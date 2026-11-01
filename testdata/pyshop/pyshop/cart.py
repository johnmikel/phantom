class Cart:
    def __init__(self):
        self.items = []

    def add_item(self, item):
        self.items.append(item)


def total(cart):
    return sum(i.price for i in cart.items)
