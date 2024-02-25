import pandas as pd
import matplotlib.pyplot as plt
from datetime import datetime
import numpy as np

from matplotlib.dates import DateFormatter, MinuteLocator

def plot_csv_data11(csv_file, battery_file_path):
    # Read the CSV files
    batteryDf = pd.read_csv(battery_file_path, usecols=[1], header=None, names=['SoC'])
    df = pd.read_csv(csv_file, usecols=[0,4], header=0, names=['timestamp', 'value'])

    # Convert the timestamp column to datetime
    df['timestamp'] = pd.to_datetime(df['timestamp'], unit='s')
    start_date = pd.to_datetime('2024-02-23 15:48:57')
    end_date = pd.to_datetime('2024-02-23 16:05:51')

    # Filter the DataFrame based on the datetime range
    filtered_df = df[(df['timestamp'] >= start_date) & (df['timestamp'] <= end_date)]
    resampled_df = filtered_df.set_index('timestamp').resample('5S').first().reset_index()

    # Create the subplots for timestamp vs value (ax1) and float numbers (ax2)
    fig, (ax1, ax2) = plt.subplots(2, 1, figsize=(12, 8))

    # Plot for the first DataFrame with timestamp vs value
    '''
    ax2.set_xlabel('Time')
    ax2.set_ylabel('Value')
    ax2.set_title('Value vs Time')

    ax2.xaxis.set_major_locator(MinuteLocator(interval=1))
    ax2.xaxis.set_major_formatter(DateFormatter('%H:%M:%S'))
    ax2.yaxis.set_ticks(np.arange(0, 9))
    ax2.tick_params(axis='x', rotation=45)  # Rotate x-axis labels for better readability
    ax2.plot(resampled_df.index, resampled_df['value'], linestyle='-')
    '''


    # Plot for the second DataFrame with float numbers
    ax1.plot(range(len(batteryDf)), batteryDf['SoC'], linestyle='-', color='red')  # Example data, adjust as needed
    ax1.xaxis.set_ticks(np.arange(0, 17))
    ax1.set_ylabel('Float Value')
    ax1.set_xlabel('Time')

    ax2.set_xlabel('Time')
    ax2.set_ylabel('Value')
    ax2.set_title('Value vs Time')

    ax2.xaxis.set_major_locator(MinuteLocator(interval=1))
    ax2.xaxis.set_major_formatter(DateFormatter('%H:%M:%S'))

    ax2.yaxis.set_ticks(np.arange(0, 9))
    ax2.tick_params(axis='x', rotation=45)  # Rotate x-axis labels for better readability
    ax2.plot(resampled_df.index, resampled_df['value'], linestyle='-')

    plt.tight_layout()
    plt.show()


def plot_csv_data(csv_file, battery_file_path):
    # Read the CSV file

    batteryDf = pd.read_csv(battery_file_path, usecols=[1], header=None, names=['SoC']).dropna()

    df = pd.read_csv(csv_file, usecols=[0,4], header=0, names=['timestamp', 'value'])

    # Convert the timestamp column to datetime
    df['timestamp'] = pd.to_datetime(df['timestamp'], unit='s')
    start_date = pd.to_datetime('2024-02-23 15:48:57')
    end_date = pd.to_datetime('2024-02-23 16:05:51')

    # Filter the DataFrame based on the datetime range
    filtered_df = df[(df['timestamp'] >= start_date) & (df['timestamp'] <= end_date)]
    seconds_resample = '5'
    resampled_df = filtered_df.set_index('timestamp').resample(seconds_resample + 'S').first().reset_index()

    x_tick_gap = 60/int(seconds_resample)
    print(x_tick_gap)
    num_x_values = len(batteryDf['SoC'])
    x_values_list = np.arange(0,num_x_values)
    x_values_list_string = str(x_values_list)
    print(num_x_values)
    print(x_values_list)
    #print(type(x_values_list_string[0]))
    # Create the subplots for timestamp vs value (ax1) and float numbers (ax2)
    fig, (ax1, ax2) = plt.subplots(2, 1, figsize=(12, 8))
    #ax1.xaxis.set_ticks(x_values_list)

    ax1.set_xlabel('Time')
    ax1.set_ylabel('Value')
    ax1.set_title('Value vs Time')

    ax1_x_values = len(resampled_df['timestamp'])
    print(ax1_x_values)
    ax1.plot(np.arange(0,ax1_x_values), resampled_df['value'], linestyle='-')


    ax1.set_xticks([])

    ticks_minutes = np.arange(0,x_tick_gap*num_x_values,x_tick_gap)
    ax1.set_xticks(ticks_minutes, x_values_list)

    #labels = ax1.get_xticklabels()
    #ticks = ax1.get_xticks()
    #for label, tick in zip(labels, ticks):
    #    if (tick%12)==0:
    #        label.set_name(8)
    #        label.set_color('r')

    #ax1.xticks(np.arange(0, 16, step=17))


    # Create the second subplot for the float numbers
    ax2.plot(np.arange(0,num_x_values), batteryDf['SoC'], linestyle='-', color='red')  # Example data, adjust as needed

    #ax1.set_xticks(x_values_list)
    #ax2.set_xticks(x_values_list)
    #x_tick_labels = [str(x) for x in x_values_list]
    #ax1.set_xticklabels(x_tick_labels)
    #ax2.set_xticklabels(x_tick_labels)

    ax2.xaxis.set_ticks(np.arange(0,num_x_values))
    #ax1.xaxis.set_ticks(np.arange(0,num_x_values))

    #ax2.xaxis.set_ticks(np.arange(0,resampled_df.index))

    #plt.tight_layout()
    plt.show()


def plot_csv_data1(csv_file):
    # Read the CSV file

    df = pd.read_csv(csv_file, usecols=[0,4], header=0, names=['timestamp', 'value'])

    # Convert the timestamp column to integer (milliseconds)
    df['timestamp'] = pd.to_datetime(df['timestamp'], unit='s')
    start_date = pd.to_datetime('2024-02-23 15:48:57')
    end_date = pd.to_datetime(' 2024-02-23 16:05:51')

    # Filter the DataFrame based on the datetime range
    filtered_df = df[(df['timestamp'] >= start_date) & (df['timestamp'] <= end_date)]
    resampled_df = filtered_df.set_index('timestamp').resample('5S').first().reset_index()


    # Plot the data
    plt.figure(figsize=(12, 4))
    plt.plot(resampled_df['timestamp'], resampled_df['value'], linestyle='-')

    # Set labels and title
    plt.xlabel('Time')
    plt.ylabel('Value')
    plt.title('Value vs Time')

    # Format x-axis as hh:mm:ss
    plt.gca().xaxis.set_major_locator(MinuteLocator(interval=1))  # Set the interval to 1 minute
    plt.gca().xaxis.set_major_formatter(DateFormatter('%H:%M:%S'))

    # Rotate x-axis labels for better readability
    plt.xticks(rotation=45)
    plt.yticks(np.arange(0,9))

    xticks_locations, xticks_labels = plt.xticks()
    num_xticks = len(xticks_locations)
    #plt.xticks(np.arange(0, num_xticks))


    # Show plot
    plt.tight_layout()
    plt.show()

def main():
    # Provide the path to your CSV file
    csv_file = '../locust_stats/euler/test_euler_stats_history.csv'
    battery_file = '../locust_stats/euler/battery_stats.csv'

    plot_csv_data(csv_file,battery_file)
    #plot_csv_data1(csv_file)
    #plot_csv_data11(csv_file,battery_file)

if __name__ == "__main__":
    main()
