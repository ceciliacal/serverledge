import matplotlib
#import tkinter as tk
#matplotlib.use('TkAgg')  # Use a suitable backend for interactive display

import pandas as pd
import numpy as np
import matplotlib.pyplot as plt

def plot_csv_data(file_paths):
    # Initialize the plot
    plt.figure(figsize=(10, 6))

    labels = ['Euler light', 'Euler baseline', 'Machine Learning light', 'Machine Learning baseline']

    # Iterate over each file
    for i, file_path in enumerate(file_paths):
        # Read the CSV file, extracting only the second column
        df = pd.read_csv(file_path, usecols=[1], header=None, names=['SoC'])

        # Create an array representing time in minutes
        time_minutes = [i for i in range(len(df))]
        #time_minutes = [i / 60.0 for i in range(len(df))]

        # Plot the data
        plt.plot(time_minutes, df['SoC'], marker='o', linestyle='-', label=labels[i])


    plt.xticks(np.arange(0,25))
    plt.yticks(np.arange(0,101,step=10))

    # Set labels and title
    plt.xlabel('Time (minutes)')
    plt.ylabel('Battery percentage')
    plt.title('Battery rate consumption in different function executions')

    # Show legend
    plt.legend()

    # Show plot
    plt.show()

    plt.savefig("batteries.pdf", format="PDF")

def main():
    # Define the file paths for the CSV files
    file_paths = ['../locust_stats/euler/battery_stats.csv', '../locust_stats/euler_standard/battery_stats.csv', '../locust_stats/ml/battery_stats.csv', '../locust_stats/ml_standard/battery_stats.csv']
    plot_csv_data(file_paths)

if __name__ == "__main__":
    main()
