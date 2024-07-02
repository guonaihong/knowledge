
### Google SRE过载保护算法

$$
\max \left( 0, \frac{\text{requests} - K \times \text{accepts}}{\text{requests} + 1} \right)
$$

### python画图验证

```python
import matplotlib.pyplot as plt

requests = 1000
accepts_list = [1000, 1000, 900, 900, 900, 800, 800, 800, 700, 600, 500, 400, 300, 200, 100, 0, 100, 200, 300, 400, 500, 600, 700, 800, 900, 1000]
K_values = [0.1]
# K_values = [0.1, 0.3, 0.5, 0.7, 1,1.1, 1.2, 1.3, 1.4, 1.5, 2, 3, 4]

# Create a figure and axis
plt.figure(figsize=(12, 8))

# Since there's only one K value, we don't need to loop over K_values
K = K_values[0]
results = []
# Loop through each accepts value
for accepts in accepts_list:
    # Calculate the value according to the formula
    value = max(0, (requests - K * accepts) / (requests + 1))
    results.append(value)
# Plot the results
plt.plot(accepts_list, results, label=f'K = {K}')

# Adding labels and title
plt.xlabel('Accepts')
plt.ylabel('Value')
plt.title('Formula values for different accepts with K = 0.1')
plt.legend()
plt.grid(True)

# Show the plot
plt.show()

```
