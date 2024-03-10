import matplotlib.pyplot as plt
import numpy as np
import pandas as pd


def plot_csv_data(csv_file, battery_file_path, start_ts, end_ts, function_name, minutes_to_add: int):
    # Read the CSV file
    battery_df = pd.read_csv(battery_file_path, usecols=[1], header=None, names=['SoC']).dropna()

    df = pd.read_csv(csv_file, usecols=[0, 4, 5], header=0, names=['timestamp', 'requests_sec', 'failures_sec'])

    # Convert the timestamp column to datetime
    df['timestamp'] = pd.to_datetime(df['timestamp'], unit='s')
    start_date = pd.to_datetime(start_ts)
    end_date = pd.to_datetime(end_ts)

    # Filter the DataFrame based on the datetime range
    filtered_df = df[(df['timestamp'] >= start_date) & (df['timestamp'] <= end_date)]
    seconds_resample = '11'
    resampled_df = filtered_df.set_index('timestamp').resample(seconds_resample + 'S').first().reset_index()

    fig = plt.figure(figsize=(7, 4.3))
    #fig = plt.figure(figsize=(11.69, 8.27))

    ax1 = fig.add_subplot(111, label="1")
    ax2 = fig.add_subplot(111, label="2", frame_on=False)

    ax1.tick_params(axis='x', labelsize=15)
    ax1.tick_params(axis='y', labelsize=15)
    ax2.tick_params(axis='y', labelsize=15)
    ax1.set_xlabel('Time [minutes]', fontsize=15, labelpad=0.5)
    ax1.set_ylabel('Throughput [req/sec]', fontsize=15, labelpad=1.5)

    ax1_x_values = len(resampled_df['timestamp'])
    print(ax1_x_values)

    if "baseline" in function_name:
        rows_to_add = int((60 / int(seconds_resample)) * minutes_to_add)
        resampled_df = resampled_df.reindex(range(ax1_x_values + rows_to_add + 1), fill_value=0)
        ax1_x_values = len(resampled_df['timestamp'])

    ax1.plot(np.arange(0, ax1_x_values), resampled_df['requests_sec'] - resampled_df['failures_sec'], linestyle='-',
             label="Throughput")
    ax1.set_xlim(left=0)
    ax1.set_xticks([])


    x_tick_gap = (60 / int(seconds_resample))
    num_x_values = len(battery_df['SoC'])
    x_values_list = np.arange(0, num_x_values)
    
    ticks_minutes = np.arange(0, x_tick_gap * num_x_values, x_tick_gap)


    ax1.set_xticks(ticks_minutes, x_values_list)

    if "ml" in function_name:
        ax1.locator_params(axis='x', nbins=8) #con ml è 8

    ax1.locator_params(axis='x', nbins=9) #con ml è 8
    ax1.set_ylim(bottom=0)
    #ax1.set_yticks(np.arange(0,2.1,0.5))

    ax2.plot(np.arange(0, num_x_values), battery_df['SoC'], linestyle='--', color='red', label="SoC")

    ax2.set_ylabel('Battery [%]', fontsize=15, labelpad=0.5)
    ax2.yaxis.tick_right()
    ax2.yaxis.set_label_position('right')
    ax2.set_xticks([])
    ax2.set_yticks(np.arange(0, 101.0, step=20))
    ax2.set_ylim(bottom=0)

    h1, l1 = ax1.get_legend_handles_labels()
    h2, l2 = ax2.get_legend_handles_labels()
    ax1.legend(h1 + h2, l1 + l2, fontsize=10, loc=0)

    #plt.show()
    plt.savefig(function_name+".pdf", format="PDF")


def main():
    # Provide the path to your CSV file
    csv_file_euler = '../locust_stats/euler/test_euler_stats_history.csv'
    battery_file_euler = '../locust_stats/euler/battery_stats.csv'
    start_ts_euler = '2024-02-23 15:48:57'
    end_ts_euler = '2024-02-23 16:05:51'

    csv_file_eulerst = '../locust_stats/euler_standard/test_euler_st_stats_history.csv'
    battery_file_eulerst = '../locust_stats/euler_standard/battery_stats.csv'
    start_ts_eulerst = '2024-02-23 16:23:32'
    end_ts_eulerst = '2024-02-23 16:35:29'

    csv_file_ml = '../locust_stats/ml/test_ml_stats_history.csv'
    battery_file_ml = '../locust_stats/ml/battery_stats.csv'
    start_ts_ml = '2024-02-26 11:36:40'
    end_ts_ml = '2024-02-26 12:11:35'

    csv_file_mlst = '../locust_stats/ml_standard/test_ml_st_stats_history.csv'
    battery_file_mlst = '../locust_stats/ml_standard/battery_stats.csv'
    start_ts_mlst = '2024-02-26 11:04:12'
    end_ts_mlst = '2024-02-26 11:29:07'

    plot_csv_data(csv_file_euler,battery_file_euler,start_ts_euler,end_ts_euler,"euler_light",0)
    #plot_csv_data(csv_file_eulerst,battery_file_eulerst,start_ts_eulerst,end_ts_eulerst,"euler_baseline",5)
    #plot_csv_data(csv_file_ml, battery_file_ml, start_ts_ml, end_ts_ml, "ml_light", 0)
    #plot_csv_data(csv_file_mlst,battery_file_mlst,start_ts_mlst,end_ts_mlst, "ml_baseline",10)


if __name__ == "__main__":
    main()
