import { ShoppingCart } from 'lucide-react';
import React from 'react';
import { Area,AreaChart,Bar,BarChart,CartesianGrid,Cell,ResponsiveContainer,Tooltip,XAxis,YAxis } from 'recharts';
import type { DashboardChartPoint } from './state';

/** 趋势图组件的输入参数。 */
export type DashboardTrendChartProps = {
  /** 趋势图的日期与营收数据点。 */
  chartData: DashboardChartPoint[];
  /** 当前选择范围的中文名称。 */
  selectedRangeLabel: string;
  /** 当前范围的营收总额。 */
  totalAmount: number;
};

/** 读取设计系统颜色变量。 */
const cssColor = (token: string, alpha?: number): string => (
  alpha === undefined ? `rgb(var(--color-${token}))` : `rgb(var(--color-${token}) / ${alpha})`
);

/** 单日数据使用紧凑的居中柱状图，避免稀疏数据被拉到宽画布边缘。 */
const SinglePointRevenue: React.FC<{ point: DashboardChartPoint }> = ({ point }) => (
  <div className="relative flex h-full items-end justify-center px-8 pb-9 pt-3">
    <div className="pointer-events-none absolute inset-x-8 bottom-9 top-3 flex flex-col justify-between border-y border-dashed border-slate-100 py-1">
      <span className="text-xs font-medium text-slate-400">¥{Number(point.amount).toFixed(2)}</span>
      <span className="self-end text-xs font-medium text-slate-300">单日营收</span>
    </div>
    <div className="relative z-10 flex h-full w-28 flex-col items-center justify-end gap-2">
      <span className="rounded-full bg-brand-50 px-3 py-1 text-sm font-extrabold tabular-nums text-brand-800">
        ¥{Number(point.amount).toFixed(2)}
      </span>
      <div className="h-28 w-16 rounded-t-2xl bg-brand shadow-brand-soft" />
      <span className="text-sm font-bold text-slate-700">{point.name}</span>
    </div>
  </div>
);

/** 展示 Dashboard 营收趋势，并根据数据点数量选择柱状图或面积图。 */
export const DashboardTrendChart: React.FC<DashboardTrendChartProps> = ({ chartData, selectedRangeLabel, totalAmount }) => (
  <div className="ios-card rounded-xl p-6 sm:p-7">
    <div className="mb-5">
      <h3 className="text-xl font-bold text-gray-900">营收趋势分析</h3>
      <p className="text-sm text-gray-400 mt-1">{selectedRangeLabel}的销售额走势</p>
    </div>
    <div className="dashboard-revenue-chart h-[230px] w-full sm:h-[250px]">
      {chartData.length === 0 || totalAmount === 0 ? (
        <div className="h-full flex flex-col items-center justify-center text-gray-400">
          <ShoppingCart className="w-16 h-16 mb-4 opacity-20" />
          <p className="text-lg font-medium">暂无营收数据</p>
          <p className="text-sm mt-2">所选时间范围内暂无订单记录</p>
        </div>
      ) : chartData.length === 1 ? (
        <SinglePointRevenue point={chartData[0]} />
      ) : chartData.length === 2 ? (
        // 数据点少于等于2个时使用美化柱状图。
        <ResponsiveContainer width="100%" height="100%">
          <BarChart data={chartData} margin={{ top: 30, right: 20, left: 0, bottom: 30 }} barCategoryGap="45%">
            <XAxis dataKey="name" axisLine={false} tickLine={false} padding={{ left: 0, right: 0 }} tick={{ fill: cssColor('neutral-700'), fontSize: 14, fontWeight: 600 }} dy={10} />
            <YAxis axisLine={false} tickLine={false} tick={{ fill: cssColor('neutral-400'), fontSize: 13, fontWeight: 500 }} tickFormatter={
              // value 是纵轴刻度的原始金额。
              value => `¥${value}`
            } />
            <Tooltip
              contentStyle={{ backgroundColor: cssColor('white'), borderRadius: '8px', border: `1px solid ${cssColor('neutral-200')}`, boxShadow: 'var(--shadow-xl)', padding: '12px 16px' }}
              labelStyle={{ color: cssColor('neutral-500'), fontWeight: 500 }}
              itemStyle={{ color: cssColor('brand'), fontWeight: 600 }}
              cursor={{ fill: cssColor('brand', 0.08) }}
              formatter={
                // value 是提示框当前数据点的原始金额。
                value => [`¥${Number(value).toFixed(2)}`, '营收']
              }
            />
            <Bar dataKey="amount" fill={cssColor('brand')} maxBarSize={72} radius={[12, 12, 0, 0]} activeBar={false} stroke="none" strokeWidth={0}>
              {chartData.map(
                // index 用于生成稳定的图表扇区键。
                (_, index) => <Cell key={`cell-${index}`} fill={cssColor('brand')} stroke="none" strokeWidth={0} />,
              )}
            </Bar>
          </BarChart>
        </ResponsiveContainer>
      ) : (
        // 数据点多于2个时使用面积图。
        <ResponsiveContainer width="100%" height="100%">
          <AreaChart data={chartData} margin={{ top: 10, right: 10, left: -20, bottom: 0 }}>
            <defs>
              <linearGradient id="colorAmount" x1="0" y1="0" x2="0" y2="1">
                <stop offset="5%" stopColor={cssColor('brand')} stopOpacity={0.5} />
                <stop offset="95%" stopColor={cssColor('brand')} stopOpacity={0} />
              </linearGradient>
            </defs>
            <XAxis dataKey="name" axisLine={false} tickLine={false} tick={{ fill: cssColor('neutral-400'), fontSize: 13, fontWeight: 500 }} dy={15} />
            <YAxis axisLine={false} tickLine={false} tick={{ fill: cssColor('neutral-400'), fontSize: 13, fontWeight: 500 }} />
            <CartesianGrid vertical={false} stroke={cssColor('neutral-100')} strokeDasharray="3 3" />
            <Tooltip
              contentStyle={{ backgroundColor: cssColor('white'), borderRadius: '8px', border: `1px solid ${cssColor('neutral-200')}`, boxShadow: 'var(--shadow-xl)', padding: '12px 16px' }}
              labelStyle={{ color: cssColor('neutral-500'), fontWeight: 500 }}
              itemStyle={{ color: cssColor('brand'), fontWeight: 600 }}
              cursor={{ stroke: cssColor('brand'), strokeWidth: 2, strokeDasharray: '4 4' }}
            />
            <Area type="monotone" dataKey="amount" stroke={cssColor('brand')} strokeWidth={4} fillOpacity={1} fill="url(#colorAmount)" activeDot={{ r: 8, fill: cssColor('white'), stroke: cssColor('brand'), strokeWidth: 2 }} />
          </AreaChart>
        </ResponsiveContainer>
      )}
    </div>
  </div>
);
