import matplotlib
#import tkinter as tk
#matplotlib.use('TkAgg')  # Use a suitable backend for interactive display

import pandas as pd
import numpy as np
import matplotlib.pyplot as plt

def plot_csv_data(file_paths):
    # Initialize the plot
    plt.figure(figsize=(10, 6))

    # Iterate over each file
    for i, file_path in enumerate(file_paths):
        # Read the CSV file, extracting only the second column
        df = pd.read_csv(file_path, usecols=[1], header=None, names=['SoC'])

        # Create an array representing time in minutes
        time_minutes = [i for i in range(len(df))]
        #time_minutes = [i / 60.0 for i in range(len(df))]

        # Plot the data
        plt.plot(time_minutes, df['SoC'], marker='o', linestyle='-', label=f'File {i+1}')


    plt.xticks(np.arange(0,25))
    plt.yticks(np.arange(0,101,step=10))

    # Set labels and title
    plt.xlabel('Time (minutes)')
    plt.ylabel('Battery percentage')
    plt.title('Float Value vs Time')

    # Show legend
    plt.legend()

    # Show plot
    plt.show()

    #todo. plt.save

def main():
    # Define the file paths for the CSV files
    file_paths = ['../locust_stats/euler/battery_stats.csv', '../locust_stats/euler_standard/battery_stats.csv', '../locust_stats/ml/battery_stats.csv', '../locust_stats/ml_standard/battery_stats.csv']
    plot_csv_data(file_paths)

if __name__ == "__main__":
    main()
