from typing import Literal
from langgraph.prebuilt import ToolNode
from langgraph.graph import StateGraph, START, END
from langchain_core.messages import SystemMessage
from pydantic import BaseModel, Field

from state import GraphState
from llm_setup import get_llm
from tools import SUPPORT_TOOLS

# Initialize LLMs
# router_llm is used for intent categorization without tools
router_llm = get_llm(temperature=0.0)
# agent_llm is bound with tools so it can decide to execute them
agent_llm = get_llm(temperature=0.0).bind_tools(SUPPORT_TOOLS)

class IntentCategory(BaseModel):
    category: str = Field(description="The category of the user's issue: 'billing', 'technical', or 'general'")

def categorize_intent(state: GraphState):
    """Categorizes the user's query using Structured Output."""
    messages = state["messages"]
    user_query = messages[-1].content
    
    # We use structured output to strictly force the LLM to return a Pydantic object
    structured_llm = router_llm.with_structured_output(IntentCategory)
    
    prompt = f"Categorize the following customer query: {user_query}"
    response = structured_llm.invoke(prompt)
    
    return {"category": response.category}

def route_category(state: GraphState) -> Literal["handle_specific", "handle_general"]:
    """Routing function that determines the next graph node based on the categorization."""
    if state["category"] in ["billing", "technical"]:
        return "handle_specific"
    else:
        return "handle_general"

def handle_specific_issue(state: GraphState):
    """Handles an issue by allowing the LLM to call tools."""
    messages = state["messages"]
    system_prompt = SystemMessage(
        content="You are a helpful customer support agent. "
                "You can process refunds and diagnose technical issues using the provided tools. "
                "Be concise. Make sure you use your tools where appropriate."
    )
    
    # Pass the conversation history so the LLM knows context and tool execution results
    response = agent_llm.invoke([system_prompt] + messages)
    
    # Appending the result. If it's a tool_call, LangGraph will see it.
    return {"messages": [response]}

def should_continue(state: GraphState) -> Literal["tools", END]:
    """Determines if the agent needs to physically execute a tool or is done."""
    last_message = state["messages"][-1]
    # If the LLM requested to run a tool, transition to 'tools' node
    if hasattr(last_message, "tool_calls") and last_message.tool_calls:
        return "tools"
    # Otherwise, it has given the final text response
    return END

def handle_general_issue(state: GraphState):
    """Handles generic questions without tool access."""
    messages = state["messages"]
    system_prompt = SystemMessage(
        content="You are a friendly customer service AI. "
                "Answer the general query politely, but clearly state you cannot process account changes or check specific systems."
    )
    
    response = router_llm.invoke([system_prompt] + messages)
    return {"messages": [response]}

# ----------------- Build the StateGraph -----------------
workflow = StateGraph(GraphState)

# 1. Add Nodes
workflow.add_node("categorize", categorize_intent)
workflow.add_node("handle_specific", handle_specific_issue)
workflow.add_node("handle_general", handle_general_issue)

# ToolNode is a pre-built LangGraph node that receives a list of tools, 
# executes the tool_calls from the last AIMessage, and appends ToolMessages.
tool_node = ToolNode(SUPPORT_TOOLS)
workflow.add_node("tools", tool_node)

# 2. Add Edges
workflow.add_edge(START, "categorize")

# Conditional routing from categorize based on intent
workflow.add_conditional_edges(
    "categorize",
    route_category
)

# If it's general, it's a terminal node
workflow.add_edge("handle_general", END)

# After executing a tool, always go back to the specific_issue handler
# so the LLM can interpret the tool output and give a final answer
workflow.add_edge("tools", "handle_specific")

# Conditional edges from handle_specific: Either run more tools, or end.
workflow.add_conditional_edges(
    "handle_specific",
    should_continue,
)

# 3. Compile the graph
agent_app = workflow.compile()
