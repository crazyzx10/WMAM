require('dotenv').config();
const https = require('https');
const mysql = require('mysql2/promise');

const CONFIG = {
    DB: {
        host: process.env.DB_HOST,
        port: parseInt(process.env.DB_PORT),
        user: process.env.DB_USER,
        password: process.env.DB_PASSWORD,
        database: process.env.DB_DATABASE
    },
    API_BASE: process.env.API_BASE,
    START_DATE: process.env.START_DATE
};

function validateConfig() {
    const errors = [];
    
    if (!CONFIG.DB.host) errors.push('❌ 缺少数据库主机配置 (DB_HOST)');
    if (!CONFIG.DB.port || isNaN(CONFIG.DB.port)) errors.push('❌ 缺少或无效的数据库端口配置 (DB_PORT)');
    if (!CONFIG.DB.user) errors.push('❌ 缺少数据库用户名配置 (DB_USER)');
    if (!CONFIG.DB.password) errors.push('❌ 缺少数据库密码配置 (DB_PASSWORD)');
    if (!CONFIG.DB.database) errors.push('❌ 缺少数据库名配置 (DB_DATABASE)');
    if (!CONFIG.API_BASE) errors.push('❌ 缺少 API 地址配置 (API_BASE)');
    if (!CONFIG.START_DATE) errors.push('❌ 缺少起始日期配置 (START_DATE)');
    
    const miniPrograms = [];
    let index = 1;
    while (process.env[`MINI_PROGRAM_${index}_NAME`]) {
        miniPrograms.push({
            name: process.env[`MINI_PROGRAM_${index}_NAME`],
            appid: process.env[`MINI_PROGRAM_${index}_APPID`],
            appsecret: process.env[`MINI_PROGRAM_${index}_APPSECRET`]
        });
        index++;
    }
    
    if (miniPrograms.length === 0) {
        errors.push('❌ 缺少小程序配置，请在 .env 文件中配置至少一个 MINI_PROGRAM');
    } else {
        for (let i = 0; i < miniPrograms.length; i++) {
            const program = miniPrograms[i];
            if (!program.name) errors.push(`❌ 小程序 ${i + 1} 缺少名称配置 (MINI_PROGRAM_${i + 1}_NAME)`);
            if (!program.appid) errors.push(`❌ 小程序 ${i + 1} 缺少 AppID 配置 (MINI_PROGRAM_${i + 1}_APPID)`);
            if (!program.appsecret) errors.push(`❌ 小程序 ${i + 1} 缺少 AppSecret 配置 (MINI_PROGRAM_${i + 1}_APPSECRET)`);
        }
    }
    
    return { valid: errors.length === 0, errors, miniPrograms };
}

const configResult = validateConfig();
if (!configResult.valid) {
    console.log('\n========================================');
    console.log('            配置验证失败');
    console.log('========================================');
    console.log('');
    configResult.errors.forEach(error => console.error(error));
    console.log('');
    console.log('请检查并完善 .env 文件中的配置');
    console.log('========================================\n');
    process.exit(1);
}

const MINI_PROGRAMS = configResult.miniPrograms;

console.log('========================================');
console.log('            ⚠️  安全提醒');
console.log('========================================');
console.log('');
console.log('🔒 敏感信息保护：');
console.log('   - .env 文件已被 .gitignore 保护');
console.log('   - 请确保不要将真实凭证提交到版本控制');
console.log('   - AppID 和 AppSecret 是敏感信息，请妥善保管');
console.log('');
console.log('📝 检查清单：');
console.log('   ✓ .env 文件已排除在 Git 之外');
console.log('   ✓ 确保生产环境使用安全的密钥管理');
console.log('');
console.log('========================================\n');

const AD_SLOT_NAMES = {
    'SLOT_ID_WEAPP_VIDEO_BEGIN': '视频贴片',
    'SLOT_ID_WEAPP_INTERSTITIAL': '插屏',
    'SLOT_ID_WEAPP_BANNER': 'Banner',
    'SLOT_ID_WEAPP_REWARD_VIDEO': '激励视频',
    'SLOT_ID_WEAPP_TEMPLATE': '原生模版',
    'SLOT_ID_WEAPP_COVER': '封面'
};

const AD_STATUS_NAMES = {
    'AD_UNIT_STATUS_ON': '正常',
    'AD_UNIT_STATUS_OFF': '暂停'
};

const tokenCaches = new Map();

function cleanExpiredTokens() {
    const now = Date.now();
    let cleanedCount = 0;
    tokenCaches.forEach((cache, key) => {
        if (now >= cache.expireTime) {
            tokenCaches.delete(key);
            cleanedCount++;
        }
    });
    if (cleanedCount > 0) {
        console.log(`[Token缓存] 已清理 ${cleanedCount} 个过期缓存`);
    }
}

setInterval(cleanExpiredTokens, 5 * 60 * 1000);

function httpGet(url) {
    return new Promise((resolve, reject) => {
        const req = https.get(url, res => {
            let data = '';
            res.on('data', chunk => data += chunk);
            res.on('end', () => {
                try { resolve(JSON.parse(data)); }
                catch (e) { reject(new Error('JSON解析失败')); }
            });
        });
        req.setTimeout(30000, () => {
            req.destroy();
            reject(new Error('请求超时'));
        });
        req.on('error', reject);
    });
}

function sleep(ms) { return new Promise(r => setTimeout(r, ms)); }

function fenToYuan(fen) { return (Number(fen) || 0) / 100; }

function fmtDate(d) {
    return `${d.getFullYear()}-${String(d.getMonth()+1).padStart(2,'0')}-${String(d.getDate()).padStart(2,'0')}`;
}

function fmtDateOnly(dateVal) {
    if (!dateVal) return null;
    if (typeof dateVal === 'string') {
        return dateVal.substring(0, 10);
    }
    return fmtDate(new Date(dateVal));
}

function formatDuration(ms) {
    const seconds = Math.floor(ms / 1000);
    const minutes = Math.floor(seconds / 60);
    const hours = Math.floor(minutes / 60);

    if (hours > 0) {
        return `${hours} 小时 ${minutes % 60} 分钟`;
    } else if (minutes > 0) {
        return `${minutes} 分钟 ${seconds % 60} 秒`;
    } else {
        return `${seconds} 秒`;
    }
}

async function getToken(appid, appsecret) {
    const now = Date.now();
    const cache = tokenCaches.get(appid);
    if (cache && now < cache.expireTime - 300000) {
        return cache.token;
    }

    cleanExpiredTokens();

    const url = `https://api.weixin.qq.com/cgi-bin/token?grant_type=client_credential&appid=${appid}&secret=${appsecret}`;
    console.log(`正在获取 ${appid} 的 access_token...`);
    const data = await httpGet(url);

    if (!data.access_token) {
        throw new Error(`获取Token失败: ${JSON.stringify(data)}`);
    }

    tokenCaches.set(appid, {
        token: data.access_token,
        expireTime: now + (data.expires_in * 1000)
    });
    console.log('Token 获取成功');
    return data.access_token;
}

async function fetchDataWithRetry(token, action, appid, appsecret, extraParams = {}) {
    let currentToken = token;
    let retryCount = 0;
    let tokenRefreshCount = 0;
    const maxRetries = 3;
    const maxTokenRefresh = 5;

    while (retryCount < maxRetries) {
        try {
            const params = new URLSearchParams({
                access_token: currentToken,
                action,
                page: '1',
                page_size: '90',
                ...Object.fromEntries(
                    Object.entries(extraParams).map(([k, v]) => [k, String(v)])
                )
            });

            const url = `${CONFIG.API_BASE}?${params}`;
            const data = await httpGet(url);

            if (data.errcode === 40001 || data.errcode === 42001) {
                if (tokenRefreshCount >= maxTokenRefresh) {
                    throw new Error('Token刷新次数超过限制');
                }
                console.log('Token已过期，正在刷新...');
                currentToken = await getToken(appid, appsecret);
                tokenRefreshCount++;
                continue;
            }

            if (data.ret !== undefined && data.ret !== 0) {
                if (data.ret === 45009) {
                    console.log('API频率限制，等待2秒后重试...');
                    await sleep(2000);
                    retryCount++;
                    continue;
                }
                throw new Error(`API错误 [${data.ret}]: ${data.err_msg || data.errmsg}`);
            }

            return data;
        } catch (err) {
            if (err.message.includes('请求超时') && retryCount < maxRetries) {
                retryCount++;
                await sleep(1000 * retryCount);
                continue;
            }
            throw err;
        }
    }

    throw new Error('重试次数超过限制');
}

async function fetchAllPagesByDateRange(token, action, startDate, endDate, appid, appsecret) {
    let allList = [];
    let page = 1;
    const pageSize = 90;
    let hasMore = true;
    let lastData = null;

    while (hasMore) {
        const data = await fetchDataWithRetry(token, action, appid, appsecret, {
            start_date: startDate,
            end_date: endDate,
            page: String(page),
            page_size: String(pageSize)
        });

        lastData = data;
        const list = data.list || [];
        allList = allList.concat(list);

        const totalNum = data.total_num || 0;
        if (page * pageSize >= totalNum || list.length < pageSize) {
            hasMore = false;
        } else {
            page++;
            await sleep(500);
        }
    }

    return { list: allList, totalNum: lastData?.total_num || allList.length };
}

async function getAdunitList(token, appid, appsecret) {
    const data = await fetchDataWithRetry(token, 'get_adunit_list', appid, appsecret);
    return data.ad_unit || [];
}

async function getSummaryData(token, startDate, endDate, appid, appsecret) {
    const result = await fetchAllPagesByDateRange(token, 'publisher_adpos_general', startDate, endDate, appid, appsecret);
    return result.list;
}

async function getDetailData(token, startDate, endDate, appid, appsecret) {
    const result = await fetchAllPagesByDateRange(token, 'publisher_adunit_general', startDate, endDate, appid, appsecret);
    return result.list;
}

async function getSettlementData(token, appid, appsecret) {
    const data = await fetchDataWithRetry(token, 'publisher_settlement', appid, appsecret);
    return data;
}

function getMonthRange(startDate, endDate) {
    const ranges = [];
    const start = new Date(startDate);
    const end = new Date(endDate);

    while (start <= end) {
        const rangeStart = fmtDate(start);
        const lastDay = new Date(start.getFullYear(), start.getMonth() + 1, 0);
        const rangeEnd = fmtDate(lastDay < end ? lastDay : end);
        ranges.push({ start: rangeStart, end: rangeEnd });
        start.setMonth(start.getMonth() + 1);
        start.setDate(1);
    }

    return ranges;
}

async function initDatabase(connection) {

    await connection.execute(`
        CREATE TABLE IF NOT EXISTS mini_program (
            名称 VARCHAR(64) NOT NULL COMMENT '小程序名称',
            小程序ID VARCHAR(32) NOT NULL COMMENT '小程序AppID',
            小程序Secret VARCHAR(64) NOT NULL COMMENT '小程序AppSecret',
            是否启用 TINYINT DEFAULT 1 COMMENT '是否启用',
            创建时间 DATETIME DEFAULT CURRENT_TIMESTAMP,
            更新时间 DATETIME DEFAULT CURRENT_TIMESTAMP ON UPDATE CURRENT_TIMESTAMP,
            PRIMARY KEY (小程序ID)
        ) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4 COMMENT='小程序配置表'
    `);

    await connection.execute(`
        CREATE TABLE IF NOT EXISTS adunit_list (
            小程序名称 VARCHAR(64) NOT NULL COMMENT '小程序名称',
            小程序ID VARCHAR(32) NOT NULL COMMENT '小程序AppID',
            广告位唯一ID VARCHAR(64) NOT NULL COMMENT '广告位唯一ID',
            广告位名称 VARCHAR(128) COMMENT '广告位名称',
            广告位类型枚举 VARCHAR(64) COMMENT '广告位类型枚举',
            广告位类型 VARCHAR(32) COMMENT '广告位类型中文名',
            广告位类型值 VARCHAR(64) COMMENT '广告位类型',
            状态 VARCHAR(32) COMMENT '状态：ON=正常，OFF=暂停',
            状态名称 VARCHAR(16) COMMENT '状态中文名',
            广告位数字ID VARCHAR(64) COMMENT '广告位数字ID',
            广告尺寸 VARCHAR(128) COMMENT '广告尺寸',
            是否允许可播放 TINYINT COMMENT '是否允许可播放',
            视频最短时长 INT COMMENT '视频最短时长(秒)',
            视频最长时长 INT COMMENT '视频最长时间(秒)',
            模版类型列表 VARCHAR(256) COMMENT '模版类型列表',
            创建时间 DATETIME DEFAULT CURRENT_TIMESTAMP,
            更新时间 DATETIME DEFAULT CURRENT_TIMESTAMP ON UPDATE CURRENT_TIMESTAMP,
            PRIMARY KEY (小程序名称, 小程序ID, 广告位唯一ID)
        ) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4 COMMENT='广告位清单表'
    `);

    await connection.execute(`
        CREATE TABLE IF NOT EXISTS publisher_adpos_general (
            小程序名称 VARCHAR(64) NOT NULL COMMENT '小程序名称',
            小程序ID VARCHAR(32) NOT NULL COMMENT '小程序AppID',
            日期 DATE NOT NULL COMMENT '日期',
            广告位类型枚举 VARCHAR(64) COMMENT '广告位类型枚举',
            广告位类型 VARCHAR(32) COMMENT '广告位类型中文名',
            广告位数字ID VARCHAR(64) COMMENT '广告位数字ID',
            成功请求次数 INT COMMENT '成功请求次数',
            曝光量 INT COMMENT '曝光量',
            曝光率 VARCHAR(10) COMMENT '曝光率(%)',
            点击量 INT COMMENT '点击量',
            点击率 VARCHAR(10) COMMENT '点击率(%)',
            总收入分 INT COMMENT '总收入(分)',
            总收入元 DECIMAL(12,2) COMMENT '总收入(元)',
            千次曝光收入分 DECIMAL(10,2) COMMENT '千次曝光收入(分)',
            千次曝光收入元 DECIMAL(10,2) COMMENT '千次曝光收入(元)',
            创建时间 DATETIME DEFAULT CURRENT_TIMESTAMP,
            PRIMARY KEY (小程序名称, 小程序ID, 日期, 广告位数字ID),
            KEY idx_appid_date (小程序ID, 日期)
        ) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4 COMMENT='广告汇总数据表'
    `);

    await connection.execute(`
        CREATE TABLE IF NOT EXISTS publisher_adunit_general (
            小程序名称 VARCHAR(64) NOT NULL COMMENT '小程序名称',
            小程序ID VARCHAR(32) NOT NULL COMMENT '小程序AppID',
            广告位唯一ID VARCHAR(64) NOT NULL COMMENT '广告位唯一ID',
            广告位名称 VARCHAR(128) COMMENT '广告位名称',
            日期 DATE NOT NULL COMMENT '日期',
            广告位类型枚举 VARCHAR(64) COMMENT '广告位类型枚举',
            广告位类型 VARCHAR(32) COMMENT '广告位类型中文名',
            广告位数字ID VARCHAR(64) COMMENT '广告位数字ID',
            成功请求次数 INT COMMENT '成功请求次数',
            曝光量 INT COMMENT '曝光量',
            曝光率 VARCHAR(10) COMMENT '曝光率(%)',
            点击量 INT COMMENT '点击量',
            点击率 VARCHAR(10) COMMENT '点击率(%)',
            总收入分 INT COMMENT '总收入(分)',
            总收入元 DECIMAL(12,2) COMMENT '总收入(元)',
            流量主收入分 INT COMMENT '流量主收入(分)',
            流量主收入元 DECIMAL(12,2) COMMENT '流量主收入(元)',
            代理商收入分 INT COMMENT '代理商收入(分)',
            千次曝光收入分 DECIMAL(10,2) COMMENT '千次曝光收入(分)',
            千次曝光收入元 DECIMAL(10,2) COMMENT '千次曝光收入(元)',
            是否智能广告 TINYINT COMMENT '是否智能广告',
            父模版类型 VARCHAR(32) COMMENT '父模版类型',
            创建时间 DATETIME DEFAULT CURRENT_TIMESTAMP,
            PRIMARY KEY (小程序名称, 小程序ID, 广告位唯一ID, 日期),
            KEY idx_appid_date (小程序ID, 日期)
        ) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4 COMMENT='广告细分数据表'
    `);

    await connection.execute(`
        CREATE TABLE IF NOT EXISTS publisher_settlement (
            小程序名称 VARCHAR(64) NOT NULL COMMENT '小程序名称',
            小程序ID VARCHAR(32) NOT NULL COMMENT '小程序AppID',
            总预估收入分 BIGINT COMMENT '总预估收入(分)',
            总预估收入元 DECIMAL(14,2) COMMENT '总预估收入(元)',
            总已结算收入分 BIGINT COMMENT '总已结算收入(分)',
            总已结算收入元 DECIMAL(14,2) COMMENT '总已结算收入(元)',
            总罚金分 BIGINT COMMENT '总罚金(分)',
            总罚金元 DECIMAL(14,2) COMMENT '总罚金(元)',
            微信云开发总预估收入分 BIGINT COMMENT '微信云开发总预估收入(分)',
            微信云开发总已结算收入分 BIGINT COMMENT '微信云开发总已结算收入(分)',
            微信云开发总罚金分 BIGINT COMMENT '微信云开发总罚金(分)',
            数据拉取日期 DATE COMMENT '数据拉取日期',
            创建时间 DATETIME DEFAULT CURRENT_TIMESTAMP,
            PRIMARY KEY (小程序名称, 小程序ID, 数据拉取日期),
            KEY idx_appid_fetch_date (小程序ID, 数据拉取日期)
        ) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4 COMMENT='结算数据表'
    `);

    await connection.execute(`
        CREATE TABLE IF NOT EXISTS fetch_log (
            小程序名称 VARCHAR(64) COMMENT '小程序名称',
            小程序ID VARCHAR(32) COMMENT '小程序AppID',
            拉取类型 VARCHAR(32) NOT NULL COMMENT '拉取类型',
            拉取日期 DATE COMMENT '拉取的数据日期',
            状态 VARCHAR(16) NOT NULL COMMENT '状态',
            记录数 INT COMMENT '拉取的记录数',
            错误信息 TEXT COMMENT '错误信息',
            创建时间 DATETIME DEFAULT CURRENT_TIMESTAMP,
            KEY idx_fetch_type (拉取类型),
            KEY idx_create_time (创建时间)
        ) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4 COMMENT='数据拉取日志表'
    `);

    console.log('数据库表初始化完成');
}

async function saveMiniProgram(connection, program) {
    await connection.execute(`
        INSERT INTO mini_program (名称, 小程序ID, 小程序Secret)
        VALUES (?, ?, ?)
        ON DUPLICATE KEY UPDATE
            名称 = VALUES(名称),
            小程序Secret = VALUES(小程序Secret),
            更新时间 = CURRENT_TIMESTAMP
    `, [program.name, program.appid, program.appsecret]);
}

async function saveAdunitList(connection, program, list) {
    if (list.length === 0) return;

    const values = [];
    for (const item of list) {
        const sizeStr = item.ad_unit_size ? JSON.stringify(item.ad_unit_size) : null;
        const templList = item.templ_type_list ? item.templ_type_list.join(',') : null;

        values.push([
            program.name,
            program.appid,
            item.ad_unit_id,
            item.ad_unit_name,
            item.ad_slot,
            AD_SLOT_NAMES[item.ad_slot] || item.ad_slot,
            item.ad_unit_type,
            item.ad_unit_status,
            AD_STATUS_NAMES[item.ad_unit_status] || item.ad_unit_status,
            item.slot_id,
            sizeStr,
            item.is_allow_playable ? 1 : 0,
            item.video_duration_min,
            item.video_duration_max,
            templList
        ]);
    }

    const placeholders = values.map(() => '(?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)').join(', ');
    
    await connection.execute(`
        INSERT INTO adunit_list (小程序名称, 小程序ID, 广告位唯一ID, 广告位名称, 广告位类型枚举, 广告位类型,
            广告位类型值, 状态, 状态名称, 广告位数字ID, 广告尺寸, 是否允许可播放,
            视频最短时长, 视频最长时长, 模版类型列表)
        VALUES ${placeholders}
        ON DUPLICATE KEY UPDATE
            广告位名称 = VALUES(广告位名称),
            广告位类型枚举 = VALUES(广告位类型枚举),
            广告位类型 = VALUES(广告位类型),
            状态 = VALUES(状态),
            状态名称 = VALUES(状态名称),
            更新时间 = CURRENT_TIMESTAMP
    `, values.flat());
}

async function saveSummaryData(connection, program, list) {
    if (list.length === 0) return;

    const values = [];
    for (const item of list) {
        values.push([
            program.name,
            program.appid,
            fmtDateOnly(item.date),
            item.ad_slot,
            AD_SLOT_NAMES[item.ad_slot] || item.ad_slot,
            item.slot_str || String(item.slot_id),
            Math.round(item.req_succ_count) || 0,
            Math.round(item.exposure_count) || 0,
            (item.exposure_rate * 100).toFixed(2) + '%',
            Math.round(item.click_count) || 0,
            (item.click_rate * 100).toFixed(2) + '%',
            item.income,
            fenToYuan(item.income),
            item.ecpm,
            fenToYuan(item.ecpm)
        ]);
    }

    const placeholders = values.map(() => '(?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)').join(', ');

    await connection.execute(`
        INSERT INTO publisher_adpos_general (小程序名称, 小程序ID, 日期, 广告位类型枚举, 广告位类型, 广告位数字ID,
            成功请求次数, 曝光量, 曝光率, 点击量, 点击率,
            总收入分, 总收入元, 千次曝光收入分, 千次曝光收入元)
        VALUES ${placeholders}
        ON DUPLICATE KEY UPDATE
            广告位类型 = VALUES(广告位类型),
            成功请求次数 = VALUES(成功请求次数),
            曝光量 = VALUES(曝光量),
            曝光率 = VALUES(曝光率),
            点击量 = VALUES(点击量),
            点击率 = VALUES(点击率),
            总收入分 = VALUES(总收入分),
            总收入元 = VALUES(总收入元),
            千次曝光收入分 = VALUES(千次曝光收入分),
            千次曝光收入元 = VALUES(千次曝光收入元)
    `, values.flat());
}

async function saveDetailData(connection, program, list) {
    if (list.length === 0) return;

    const values = [];
    for (const item of list) {
        const stat = item.stat_item;
        values.push([
            program.name,
            program.appid,
            item.ad_unit_id,
            item.ad_unit_name,
            fmtDateOnly(stat.date),
            stat.ad_slot,
            AD_SLOT_NAMES[stat.ad_slot] || stat.ad_slot,
            stat.slot_str,
            Math.round(stat.req_succ_count) || 0,
            Math.round(stat.exposure_count) || 0,
            (stat.exposure_rate * 100).toFixed(2) + '%',
            Math.round(stat.click_count) || 0,
            (stat.click_rate * 100).toFixed(2) + '%',
            stat.income,
            fenToYuan(stat.income),
            stat.publisher_income,
            fenToYuan(stat.publisher_income),
            stat.agency_income,
            stat.ecpm,
            fenToYuan(stat.ecpm),
            stat.is_smart_ads,
            stat.parent_templ_type
        ]);
    }

    const placeholders = values.map(() => '(?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)').join(', ');

    await connection.execute(`
        INSERT INTO publisher_adunit_general (小程序名称, 小程序ID, 广告位唯一ID, 广告位名称, 日期,
            广告位类型枚举, 广告位类型, 广告位数字ID, 成功请求次数, 曝光量, 曝光率,
            点击量, 点击率, 总收入分, 总收入元, 流量主收入分,
            流量主收入元, 代理商收入分, 千次曝光收入分, 千次曝光收入元, 是否智能广告, 父模版类型)
        VALUES ${placeholders}
        ON DUPLICATE KEY UPDATE
            广告位名称 = VALUES(广告位名称),
            广告位类型 = VALUES(广告位类型),
            成功请求次数 = VALUES(成功请求次数),
            曝光量 = VALUES(曝光量),
            曝光率 = VALUES(曝光率),
            点击量 = VALUES(点击量),
            点击率 = VALUES(点击率),
            总收入分 = VALUES(总收入分),
            总收入元 = VALUES(总收入元),
            流量主收入分 = VALUES(流量主收入分),
            流量主收入元 = VALUES(流量主收入元),
            代理商收入分 = VALUES(代理商收入分),
            千次曝光收入分 = VALUES(千次曝光收入分),
            千次曝光收入元 = VALUES(千次曝光收入元),
            是否智能广告 = VALUES(是否智能广告),
            父模版类型 = VALUES(父模版类型)
    `, values.flat());
}

async function saveSettlementData(connection, program, data) {
    const wyw = data.wyw_settled_summary || {};
    await connection.execute(`
        INSERT INTO publisher_settlement (小程序名称, 小程序ID, 总预估收入分, 总预估收入元,
            总已结算收入分, 总已结算收入元, 总罚金分, 总罚金元,
            微信云开发总预估收入分, 微信云开发总已结算收入分, 微信云开发总罚金分, 数据拉取日期)
        VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, CURDATE())
        ON DUPLICATE KEY UPDATE
            总预估收入分 = VALUES(总预估收入分),
            总预估收入元 = VALUES(总预估收入元),
            总已结算收入分 = VALUES(总已结算收入分),
            总已结算收入元 = VALUES(总已结算收入元),
            总罚金分 = VALUES(总罚金分),
            总罚金元 = VALUES(总罚金元),
            微信云开发总预估收入分 = VALUES(微信云开发总预估收入分),
            微信云开发总已结算收入分 = VALUES(微信云开发总已结算收入分),
            微信云开发总罚金分 = VALUES(微信云开发总罚金分),
            创建时间 = CURRENT_TIMESTAMP
    `, [
        program.name,
        program.appid,
        data.revenue_all,
        fenToYuan(data.revenue_all),
        data.settled_revenue_all,
        fenToYuan(data.settled_revenue_all),
        data.penalty_all,
        fenToYuan(data.penalty_all),
        wyw.wyw_revenue_all || 0,
        wyw.wyw_settled_revenue_all || 0,
        wyw.wyw_penalty_all || 0
    ]);
}

async function logFetch(connection, program, fetchType, fetchDate, status, totalCount, errorMessage) {
    await connection.execute(`
        INSERT INTO fetch_log (小程序名称, 小程序ID, 拉取类型, 拉取日期, 状态, 记录数, 错误信息)
        VALUES (?, ?, ?, ?, ?, ?, ?)
    `, [program.name, program.appid, fetchType, fetchDate, status, totalCount, errorMessage]);
}

async function getLatestDataDate(connection, tableName, dateColumn, appid) {
    // 白名单验证：只允许指定的表名和列名
    const allowedTables = {
        'publisher_adpos_general': '日期',
        'publisher_adunit_general': '日期'
    };
    
    if (!allowedTables[tableName] || allowedTables[tableName] !== dateColumn) {
        console.log(`警告: 不允许的表名或列名 ${tableName}.${dateColumn}，使用起始日期`);
        return CONFIG.START_DATE;
    }
    
    try {
        const [rows] = await connection.execute(`
            SELECT MAX(${dateColumn}) as latest_date FROM ${tableName} WHERE 小程序ID = ?
        `, [appid]);
        if (rows[0].latest_date) {
            const latest = new Date(rows[0].latest_date);
            latest.setDate(latest.getDate() + 1);
            return fmtDate(latest);
        }
    } catch (err) {
        console.error(`查询最新日期失败: ${err.message}`);
    }
    return CONFIG.START_DATE;
}

async function fetchProgramData(connection, program) {
    const programStartTime = Date.now();
    console.log(`\n====== 处理小程序: ${program.name} (${program.appid}) ======`);

    try {
        await saveMiniProgram(connection, program);

        const token = await getToken(program.appid, program.appsecret);

        console.log('\n----- 1. 拉取广告位清单 -----');
        const adunits = await getAdunitList(token, program.appid, program.appsecret);
        console.log(`广告位数量: ${adunits.length}`);
        if (adunits.length > 0) {
            await saveAdunitList(connection, program, adunits);
            console.log('广告位清单已保存');
        }
        await logFetch(connection, program, 'adunit_list', null, 'success', adunits.length, null);

        const today = fmtDate(new Date());

        console.log('\n----- 2. 拉取汇总数据 -----');
        const lastSummaryDate = await getLatestDataDate(connection, 'publisher_adpos_general', '日期', program.appid);
        console.log(`从 ${lastSummaryDate} 开始拉取汇总数据...`);
        const monthRanges = getMonthRange(lastSummaryDate, today);

        let totalSummaryCount = 0;
        for (const range of monthRanges) {
            try {
                const data = await getSummaryData(token, range.start, range.end, program.appid, program.appsecret);
                if (data.length > 0) {
                    await saveSummaryData(connection, program, data);
                    totalSummaryCount += data.length;
                    console.log(`  ${range.start} ~ ${range.end}: ${data.length} 条`);
                } else {
                    console.log(`  ${range.start} ~ ${range.end}: 无数据`);
                }
                await sleep(500);
            } catch (err) {
                console.error(`  ${range.start} ~ ${range.end}: 拉取失败 - ${err.message}`);
            }
        }
        await logFetch(connection, program, 'publisher_adpos_general', today, 'success', totalSummaryCount, null);
        console.log(`汇总数据拉取完成, 共 ${totalSummaryCount} 条`);

        console.log('\n----- 3. 拉取细分数据 -----');
        const lastDetailDate = await getLatestDataDate(connection, 'publisher_adunit_general', '日期', program.appid);
        console.log(`从 ${lastDetailDate} 开始拉取细分数据...`);
        const detailRanges = getMonthRange(lastDetailDate, today);

        let totalDetailCount = 0;
        for (const range of detailRanges) {
            try {
                const data = await getDetailData(token, range.start, range.end, program.appid, program.appsecret);
                if (data.length > 0) {
                    await saveDetailData(connection, program, data);
                    totalDetailCount += data.length;
                    console.log(`  ${range.start} ~ ${range.end}: ${data.length} 条`);
                } else {
                    console.log(`  ${range.start} ~ ${range.end}: 无数据`);
                }
                await sleep(500);
            } catch (err) {
                console.error(`  ${range.start} ~ ${range.end}: 拉取失败 - ${err.message}`);
            }
        }
        await logFetch(connection, program, 'publisher_adunit_general', today, 'success', totalDetailCount, null);
        console.log(`细分数据拉取完成, 共 ${totalDetailCount} 条`);

        console.log('\n----- 4. 拉取结算数据 -----');
        try {
            const settlement = await getSettlementData(token, program.appid, program.appsecret);
            await saveSettlementData(connection, program, settlement);
            await logFetch(connection, program, 'publisher_settlement', today, 'success', 1, null);
            console.log('结算数据已保存');
            console.log(`  总预估收入: ${fenToYuan(settlement.revenue_all).toFixed(2)} 元`);
            console.log(`  总已结算收入: ${fenToYuan(settlement.settled_revenue_all).toFixed(2)} 元`);
        } catch (err) {
            console.error(`结算数据拉取失败: ${err.message}`);
            await logFetch(connection, program, 'publisher_settlement', today, 'failed', 0, err.message);
        }

        const programDuration = Date.now() - programStartTime;
        console.log(`\n====== ${program.name} 数据拉取完成 ======`);
        console.log(`耗时: ${formatDuration(programDuration)}`);

    } catch (err) {
        console.error(`\n====== ${program.name} 拉取出错 ======`);
        console.error(`错误: ${err.message}`);
        await logFetch(connection, program, 'unknown', null, 'failed', 0, err.message);
        throw err;
    }
}

async function main() {
    const mainStartTime = Date.now();
    console.log('========== 微信小程序广告数据拉取开始 ==========');
    console.log(`开始时间: ${new Date().toLocaleString()}`);
    console.log(`起始日期: ${CONFIG.START_DATE}`);
    console.log(`小程序数量: ${MINI_PROGRAMS.length}`);
    console.log('');

    let connection;
    try {
        connection = await mysql.createConnection(CONFIG.DB);
        await initDatabase(connection);

        for (const program of MINI_PROGRAMS) {
            await fetchProgramData(connection, program);
        }

        const mainDuration = Date.now() - mainStartTime;
        console.log('\n========== 全部数据拉取完成 ==========');
        console.log(`完成时间: ${new Date().toLocaleString()}`);
        console.log(`总耗时: ${formatDuration(mainDuration)}`);

    } catch (err) {
        console.error('\n========== 拉取过程出错 ==========');
        console.error(`错误: ${err.message}`);
        process.exit(1);
    } finally {
        if (connection) {
            await connection.end();
        }
    }
}

main().catch(err => {
    console.error('Fatal error:', err);
    process.exit(1);
});
