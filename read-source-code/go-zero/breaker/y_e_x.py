import numpy as np
import matplotlib.pyplot as plt

# 定义 x 值
x_values = np.linspace(0, 10, 100)
# 定义 y 值为 e^(-x)
y_values = np.exp(-x_values)

# 绘制图表
plt.figure(figsize=(10, 6))
plt.plot(x_values, y_values, label='$e^{-x}$', color='blue')
plt.title("Graph of $e^{-x}$")
plt.xlabel("x")
plt.ylabel("$e^{-x}$")
plt.grid(True)
plt.legend()
plt.show()
