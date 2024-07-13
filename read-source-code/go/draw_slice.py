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

# Case 1: newLen > doublecap
oldCap = 100
newLens_case1 = list(range(250, 10000, 10))  # Ensure newLen > doublecap for all values
newCaps_case1 = [nextslicecap(newLen, oldCap) for newLen in newLens_case1]

# # Case 2: oldCap < threshold
newLens_case2 = list(range(2, 256, 1))
newCaps_case2 = [nextslicecap(newLen, newLen-1) for newLen in newLens_case2]

# Case 3: oldCap >= threshold
oldCap = 300  # Ensure oldCap >= 256
newLens_case3 = list(range(100, 10000, 10))
newCaps_case3 = [nextslicecap(newLen, oldCap) for newLen in newLens_case3]

# Plot the growth curves
plt.figure(figsize=(10, 10))

plt.subplot(3, 1, 1)
plt.plot(newLens_case1, newCaps_case1, marker='o')
plt.title("Capacity Growth Curve - Case 1: newLen > doublecap")
plt.xlabel("New Length (newLen)")
plt.ylabel("New Capacity (newcap)")
plt.grid(True)

plt.subplot(3, 1, 2)
plt.plot(newLens_case2, newCaps_case2, marker='o')
plt.title("Capacity Growth Curve - Case 2: oldCap < threshold")
plt.xlabel("New Length (newLen)")
plt.ylabel("New Capacity (newcap)")
plt.grid(True)

plt.subplot(3, 1, 3)
plt.plot(newLens_case3, newCaps_case3, marker='o')
plt.title("Capacity Growth Curve - Case 3: oldCap >= threshold")
plt.xlabel("New Length (newLen)")
plt.ylabel("New Capacity (newcap)")
plt.grid(True)

plt.tight_layout()
plt.show()
