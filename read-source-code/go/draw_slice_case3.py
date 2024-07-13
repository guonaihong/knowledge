import matplotlib.pyplot as plt

def nextslicecap(newLen, oldCap):
    newcap = oldCap
    doublecap = newcap + newcap
    if newLen > doublecap:
        return newLen
    
    threshold = 256
    if oldCap < threshold:
        return doublecap

    while True:
        newcap += (newcap + 3 * threshold) >> 2
        if newcap >= newLen:
            break

    if newcap <= 0:
        return newLen
    return newcap

# Case 3: oldCap >= threshold
newLens_case3 = list(range(256, 10000, 10))
newCaps_case3 = [nextslicecap(newLen, newLen-1) for newLen in newLens_case3]

# Plot the growth curves
plt.figure(figsize=(10, 10))

plt.subplot(1, 1, 1)
plt.plot(newLens_case3, newCaps_case3, marker='o')
plt.title("Capacity Growth Curve - Case 3: oldCap >= threshold")
plt.xlabel("New Length (newLen)")
plt.ylabel("New Capacity (newcap)")
plt.grid(True)

plt.tight_layout()
plt.show()