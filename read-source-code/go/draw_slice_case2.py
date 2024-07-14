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

# # Case 2: oldCap < threshold
newLens_case2 = list(range(2, 256, 1))
newCaps_case2 = [nextslicecap(newLen, newLen-1) for newLen in newLens_case2]


# Plot the growth curves
plt.figure(figsize=(8, 8))

plt.subplot(1, 2, 1)
plt.plot(newLens_case2, newCaps_case2, marker='o')
plt.title("Capacity Growth Curve - Case 2: oldCap < threshold")
plt.xlabel("New Length (newLen)")
plt.ylabel("New Capacity (newcap)")
plt.grid(True)


plt.tight_layout()
plt.show()
